package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonElement
import com.intellij.openapi.actionSystem.ActionManager
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.project.DumbService
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.psi.PsiDocumentManager
import com.intellij.psi.PsiManager
import java.lang.reflect.Array as JavaArray
import java.lang.reflect.Constructor
import java.lang.reflect.Field
import java.lang.reflect.Method
import java.lang.reflect.Modifier
import java.util.IdentityHashMap
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference

class ReflectiveRuntime(private val project: Project) : ReflectionPort {
    private val store = ReflectiveObjectStore()

    init {
        store.putRoot("root:project", project)
        store.putRoot("root:application", ApplicationManager.getApplication())
        store.putRoot("root:actionManager", ActionManager.getInstance())
        store.putRoot("root:fileEditorManager", FileEditorManager.getInstance(project))
        store.putRoot("root:fileDocumentManager", FileDocumentManager.getInstance())
        store.putRoot("root:psiManager", PsiManager.getInstance(project))
        store.putRoot("root:psiDocumentManager", PsiDocumentManager.getInstance(project))
        store.putRoot("root:localFileSystem", LocalFileSystem.getInstance())
        store.putRoot("root:dumbService", DumbService.getInstance(project))
        optionalRoot("root:projectRootManager", "com.intellij.openapi.roots.ProjectRootManager", "getInstance", project)
        optionalRoot("root:runManager", "com.intellij.execution.RunManager", "getInstance", project)
        optionalRoot("root:daemonCodeAnalyzer", "com.intellij.codeInsight.daemon.DaemonCodeAnalyzer", "getInstance", project)
        optionalRoot("root:inspectionProfileManager", "com.intellij.profile.codeInspection.InspectionProjectProfileManager", "getInstance", project)
        optionalRoot("root:refactoringFactory", "com.intellij.refactoring.RefactoringFactory", "getInstance", project)
    }

    override fun protocol(): Map<String, Any?> = mapOf(
        "formatVersion" to 1,
        "argumentForms" to listOf(
            "JSON primitives map to Java primitives, boxed numbers, booleans, and strings.",
            mapOf("\$ref" to "handle id returned by this server"),
            mapOf("\$class" to "fully.qualified.ClassName"),
            mapOf("\$enum" to mapOf("className" to "fully.qualified.EnumClass", "name" to "CONSTANT")),
            mapOf("\$array" to listOf("items"), "componentType" to "optional fully.qualified.Component"),
        ),
        "handleLifetime" to "Handles live until released or until the IDE project MCP service stops. root:* handles cannot be released.",
        "threading" to mapOf(
            "direct" to "java.lang.reflect.Method.invoke",
            "read" to "com.intellij.openapi.application.Application.runReadAction",
            "write" to "com.intellij.openapi.command.WriteCommandAction.runWriteCommandAction",
            "dispatchThread" to "Set dispatchThread=true to run invocation on the IDE event dispatch thread.",
        ),
    )

    override fun roots(): Map<String, Any?> = mapOf(
        "formatVersion" to 1,
        "roots" to store.roots().associateWith { store.describe(it) },
    )

    override fun classForName(className: String): Map<String, Any?> {
        val clazz = classForNameStrict(className)
        return mapOf("class" to describeClassValue(clazz), "handle" to store.put(clazz))
    }

    override fun describeClass(input: ReflectiveClassInput): Map<String, Any?> {
        val clazz = input.classNameOrRefClass()
        val methods = clazz.methodList(input.includeDeclared)
            .sortedWith(compareBy<Method> { it.name }.thenBy { it.parameterCount })
            .take(input.limit)
            .map { it.toValue() }
        val fields = clazz.fieldList(input.includeDeclared)
            .sortedBy { it.name }
            .take(input.limit)
            .map { it.toValue() }
        return mapOf(
            "class" to describeClassValue(clazz),
            "methods" to methods,
            "fields" to fields,
        )
    }

    override fun constructors(input: ReflectiveClassInput): Map<String, Any?> {
        val clazz = input.classNameOrRefClass()
        val values = clazz.constructors(input.includeDeclared)
            .sortedBy { it.parameterCount }
            .take(input.limit)
            .map { it.toValue() }
        return mapOf("class" to describeClassValue(clazz), "constructors" to values)
    }

    override fun getField(input: ReflectiveFieldInput): Map<String, Any?> {
        val target = input.targetRef.takeIf { it.isNotBlank() }?.let { store.get(it) }
        val clazz = input.className.takeIf { it.isNotBlank() }?.let { classForNameStrict(it) }
            ?: target?.javaClass
            ?: error("className or targetRef is required")
        val field = clazz.findField(input.fieldName)
        val value = field.get(target)
        return reflectiveResult(value, input.storeResult)
    }

    override fun newInstance(input: ReflectiveConstructorInput): Map<String, Any?> {
        val clazz = classForNameStrict(input.className)
        val constructor = clazz.findConstructor(input.parameterTypes, input.arguments)
        val args = constructor.parameterTypes.zip(input.arguments).map { (type, value) -> coerce(value, type) }.toTypedArray()
        val value = constructor.newInstance(*args)
        return reflectiveResult(value, input.storeResult)
    }

    override fun invoke(input: ReflectiveInvocationInput): Map<String, Any?> =
        runMaybeOnDispatchThread(input.dispatchThread) { invokeDirect(input) }

    override fun invokeReadAction(input: ReflectiveInvocationInput): Map<String, Any?> =
        runMaybeOnDispatchThread(input.dispatchThread) {
            ApplicationManager.getApplication().runReadAction<Map<String, Any?>> { invokeDirect(input) }
        }

    override fun invokeWriteCommand(input: ReflectiveInvocationInput): Map<String, Any?> =
        runMaybeOnDispatchThread(input.dispatchThread) {
            var result: Map<String, Any?>? = null
            WriteCommandAction.runWriteCommandAction(project) {
                result = invokeDirect(input)
            }
            result ?: error("Write command did not return a result")
        }

    override fun handles(): Map<String, Any?> = mapOf(
        "count" to store.refs().size,
        "handles" to store.refs().associateWith { store.describe(it) },
    )

    override fun handle(ref: String): Map<String, Any?> = store.describe(ref)

    override fun release(ref: String): Map<String, Any?> = mapOf("ref" to ref, "released" to store.release(ref))

    private fun invokeDirect(input: ReflectiveInvocationInput): Map<String, Any?> {
        val target = input.targetRef.takeIf { it.isNotBlank() }?.let { store.get(it) }
        val clazz = input.className.takeIf { it.isNotBlank() }?.let { classForNameStrict(it) }
            ?: (target as? Class<*>)
            ?: target?.javaClass
            ?: error("className or targetRef is required")
        val staticCall = target == null || target is Class<*>
        val method = clazz.findMethod(input.methodName, input.parameterTypes, input.arguments)
        val args = method.parameterTypes.zip(input.arguments).map { (type, value) -> coerce(value, type) }.toTypedArray()
        val value = method.invoke(if (staticCall) null else target, *args)
        return reflectiveResult(value, input.storeResult)
    }

    private fun ReflectiveClassInput.classNameOrRefClass(): Class<*> {
        className.takeIf { it.isNotBlank() }?.let { return classForNameStrict(it) }
        val value = ref.takeIf { it.isNotBlank() }?.let { store.get(it) } ?: error("className or ref is required")
        return (value as? Class<*>) ?: value.javaClass
    }

    private fun reflectiveResult(value: Any?, storeResult: Boolean): Map<String, Any?> = mapOf(
        "formatVersion" to 1,
        "value" to ReflectiveValueWriter(store).write(value, storeResult),
    )

    private fun optionalRoot(ref: String, className: String, methodName: String, project: Project) {
        runCatching {
            val clazz = classForNameStrict(className)
            val method = clazz.findMethod(methodName, listOf(Project::class.java.name), emptyList())
            store.putRoot(ref, method.invoke(null, project))
        }
    }

    private fun runMaybeOnDispatchThread(dispatchThread: Boolean, body: () -> Map<String, Any?>): Map<String, Any?> {
        if (!dispatchThread || ApplicationManager.getApplication().isDispatchThread) return body()
        val result = AtomicReference<Map<String, Any?>?>()
        val error = AtomicReference<Throwable?>()
        ApplicationManager.getApplication().invokeAndWait {
            try {
                result.set(body())
            } catch (t: Throwable) {
                error.set(t)
            }
        }
        error.get()?.let { throw it }
        return result.get() ?: error("Dispatch thread invocation did not return a result")
    }

    private fun classForNameStrict(className: String): Class<*> =
        when (className) {
            "boolean" -> java.lang.Boolean.TYPE
            "byte" -> java.lang.Byte.TYPE
            "char" -> java.lang.Character.TYPE
            "short" -> java.lang.Short.TYPE
            "int" -> java.lang.Integer.TYPE
            "long" -> java.lang.Long.TYPE
            "float" -> java.lang.Float.TYPE
            "double" -> java.lang.Double.TYPE
            "void" -> java.lang.Void.TYPE
            else -> Class.forName(className)
        }

    private fun Class<*>.findMethod(name: String, parameterTypes: List<String>, args: List<JsonElement>): Method {
        if (parameterTypes.isNotEmpty()) {
            return methodList(true)
                .firstOrNull { it.name == name && it.parameterTypes.map { type -> type.name } == parameterTypes }
                ?.makeAccessible()
                ?: error("Method not found: $name(${parameterTypes.joinToString()}) on ${this.name}")
        }
        return methodList(true)
            .firstOrNull { it.name == name && it.parameterCount == args.size && canCoerceAll(args, it.parameterTypes) }
            ?.makeAccessible()
            ?: error("Method not found: $name with ${args.size} argument(s) on ${this.name}")
    }

    private fun Class<*>.findConstructor(parameterTypes: List<String>, args: List<JsonElement>): Constructor<*> {
        if (parameterTypes.isNotEmpty()) {
            return constructors(true)
                .firstOrNull { it.parameterTypes.map { type -> type.name } == parameterTypes }
                ?.makeAccessible()
                ?: error("Constructor not found: ${this.name}(${parameterTypes.joinToString()})")
        }
        return constructors(true)
            .firstOrNull { it.parameterCount == args.size && canCoerceAll(args, it.parameterTypes) }
            ?.makeAccessible()
            ?: error("Constructor not found: ${this.name} with ${args.size} argument(s)")
    }

    private fun Class<*>.findField(name: String): Field {
        var current: Class<*>? = this
        while (current != null) {
            current.declaredFields.firstOrNull { it.name == name }?.let { return it.makeAccessible() }
            current = current.superclass
        }
        return getField(name).makeAccessible()
    }

    private fun canCoerceAll(args: List<JsonElement>, parameterTypes: kotlin.Array<Class<*>>): Boolean =
        args.zip(parameterTypes).all { (arg, type) -> runCatching { coerce(arg, type) }.isSuccess }

    private fun coerce(value: JsonElement, type: Class<*>): Any? {
        if (value.isJsonNull) return null
        if (value.isJsonObject) {
            val obj = value.asJsonObject
            obj.get("\$ref")?.asString?.let { return store.get(it) }
            obj.get("\$class")?.asString?.let { return classForNameStrict(it) }
            obj.get("\$enum")?.asJsonObject?.let {
                val enumClass = classForNameStrict(it.get("className").asString).asSubclass(Enum::class.java)
                return java.lang.Enum.valueOf(enumClass, it.get("name").asString)
            }
            obj.get("\$array")?.asJsonArray?.let { array ->
                val component = obj.get("componentType")?.asString?.let { classForNameStrict(it) }
                    ?: type.componentType
                    ?: Any::class.java
                return array.toList().toJavaArray(component)
            }
        }
        if (value.isJsonArray) {
            val component = type.componentType ?: Any::class.java
            return value.asJsonArray.toList().toJavaArray(component)
        }
        val primitive = value.asJsonPrimitive
        return when {
            type == String::class.java -> primitive.asString
            type == java.lang.Boolean.TYPE || type == java.lang.Boolean::class.java -> primitive.asBoolean
            type == java.lang.Integer.TYPE || type == java.lang.Integer::class.java -> primitive.asInt
            type == java.lang.Long.TYPE || type == java.lang.Long::class.java -> primitive.asLong
            type == java.lang.Float.TYPE || type == java.lang.Float::class.java -> primitive.asFloat
            type == java.lang.Double.TYPE || type == java.lang.Double::class.java -> primitive.asDouble
            type == java.lang.Short.TYPE || type == java.lang.Short::class.java -> primitive.asShort
            type == java.lang.Byte.TYPE || type == java.lang.Byte::class.java -> primitive.asByte
            type == java.lang.Character.TYPE || type == java.lang.Character::class.java -> primitive.asString.first()
            type.isEnum -> java.lang.Enum.valueOf(type.asSubclass(Enum::class.java), primitive.asString)
            else -> primitive.asString
        }
    }

    private fun List<JsonElement>.toJavaArray(component: Class<*>): Any {
        val array = JavaArray.newInstance(component, size)
        forEachIndexed { index, value -> JavaArray.set(array, index, coerce(value, component)) }
        return array
    }

    private fun Class<*>.methodList(includeDeclared: Boolean): List<Method> =
        if (includeDeclared) (methods.toList() + declaredMethods.toList()).distinctBy { it.signatureKey() } else methods.toList()

    private fun Class<*>.fieldList(includeDeclared: Boolean): List<Field> =
        if (includeDeclared) (fields.toList() + declaredFields.toList()).distinctBy { it.name } else fields.toList()

    private fun Class<*>.constructors(includeDeclared: Boolean): List<Constructor<*>> =
        if (includeDeclared) declaredConstructors.toList() else constructors.toList()

    private fun Method.signatureKey(): String = "$name(${parameterTypes.joinToString(",") { it.name }})"

    private fun <T : java.lang.reflect.AccessibleObject> T.makeAccessible(): T {
        runCatching { isAccessible = true }
        return this
    }
}

private class ReflectiveObjectStore {
    private val next = AtomicLong(1)
    private val refs = ConcurrentHashMap<String, Any>()
    private val roots = mutableSetOf<String>()
    private val identities = IdentityHashMap<Any, String>()

    @Synchronized
    fun put(value: Any): String {
        if (value is Class<*>) return putRoot("class:${value.name}", value)
        identities[value]?.let { return it }
        val ref = "obj:${next.getAndIncrement()}"
        refs[ref] = value
        identities[value] = ref
        return ref
    }

    @Synchronized
    fun putRoot(ref: String, value: Any): String {
        refs[ref] = value
        roots += ref
        identities[value] = ref
        return ref
    }

    fun get(ref: String): Any = refs[ref] ?: error("Unknown reflective handle: $ref")

    fun roots(): List<String> = roots.sorted()

    fun refs(): List<String> = refs.keys().toList().sorted()

    fun describe(ref: String): Map<String, Any?> {
        val value = get(ref)
        return mapOf(
            "\$ref" to ref,
            "root" to roots.contains(ref),
            "className" to value.javaClass.name,
            "identityHash" to System.identityHashCode(value),
            "display" to value.toString().take(500),
        )
    }

    @Synchronized
    fun release(ref: String): Boolean {
        if (roots.contains(ref)) return false
        val value = refs.remove(ref) ?: return false
        identities.remove(value)
        return true
    }
}

private class ReflectiveValueWriter(private val store: ReflectiveObjectStore) {
    fun write(value: Any?, storeResult: Boolean, depth: Int = 0): Any? {
        if (value == null || value is String || value is Number || value is Boolean) return value
        if (value is Char) return value.toString()
        if (value is Enum<*>) return mapOf("\$enum" to mapOf("className" to value.javaClass.name, "name" to value.name))
        if (value is Class<*>) return describeClassValue(value) + mapOf("\$ref" to store.put(value))
        if (depth < 2 && value.javaClass.isArray) {
            return (0 until JavaArray.getLength(value)).map { write(JavaArray.get(value, it), storeResult, depth + 1) }
        }
        if (depth < 2 && value is Iterable<*>) return value.map { write(it, storeResult, depth + 1) }
        if (depth < 2 && value is Map<*, *>) {
            return value.entries.associate { it.key.toString() to write(it.value, storeResult, depth + 1) }
        }
        if (!storeResult) {
            return mapOf(
                "className" to value.javaClass.name,
                "identityHash" to System.identityHashCode(value),
                "display" to value.toString().take(500),
            )
        }
        return store.describe(store.put(value))
    }
}

private fun describeClassValue(clazz: Class<*>): Map<String, Any?> = mapOf(
    "name" to clazz.name,
    "simpleName" to clazz.simpleName,
    "canonicalName" to clazz.canonicalName,
    "packageName" to clazz.`package`?.name,
    "modifiers" to Modifier.toString(clazz.modifiers),
    "isInterface" to clazz.isInterface,
    "isEnum" to clazz.isEnum,
    "isAnnotation" to clazz.isAnnotation,
    "superclass" to clazz.superclass?.name,
    "interfaces" to clazz.interfaces.map { it.name },
)

private fun Method.toValue(): Map<String, Any?> = mapOf(
    "name" to name,
    "declaringClass" to declaringClass.name,
    "modifiers" to Modifier.toString(modifiers),
    "returnType" to returnType.name,
    "parameterTypes" to parameterTypes.map { it.name },
)

private fun Field.toValue(): Map<String, Any?> = mapOf(
    "name" to name,
    "declaringClass" to declaringClass.name,
    "modifiers" to Modifier.toString(modifiers),
    "type" to type.name,
)

private fun Constructor<*>.toValue(): Map<String, Any?> = mapOf(
    "declaringClass" to declaringClass.name,
    "modifiers" to Modifier.toString(modifiers),
    "parameterTypes" to parameterTypes.map { it.name },
)
