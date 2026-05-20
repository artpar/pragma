package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser

fun JsonObject.stringArg(name: String, defaultValue: String = ""): String {
    val value = get(name) ?: return defaultValue
    return if (value.isJsonNull) defaultValue else value.asString
}

fun JsonObject.intArg(name: String, defaultValue: Int = 0): Int {
    val value = get(name) ?: return defaultValue
    return if (value.isJsonNull) defaultValue else value.asInt
}

fun JsonObject.boolArg(name: String, defaultValue: Boolean = false): Boolean {
    val value = get(name) ?: return defaultValue
    return if (value.isJsonNull) defaultValue else value.asBoolean
}

fun JsonObject.stringListArg(name: String): List<String> {
    val value = get(name) ?: return emptyList()
    if (!value.isJsonArray) return emptyList()
    return value.asJsonArray.mapNotNull { if (it.isJsonNull) null else it.asString }
}

fun JsonObject.jsonListArg(name: String): List<JsonElement> {
    val value = get(name) ?: return emptyList()
    if (!value.isJsonArray) return emptyList()
    return value.asJsonArray.toList()
}

fun JsonObject.jsonArg(name: String): JsonElement? {
    val value = get(name)?.takeUnless { it.isJsonNull } ?: return null
    return value.parseJsonStringValue()
}

internal fun validateReplacementExpectedText(args: JsonObject, current: String): Map<String, Any?>? {
    val expected = args.stringArgOrNull("expectedText") ?: args.stringArgOrNull("oldText")
    if (expected == null) {
        return replacementError(
            "missing_expected_text",
            "Replacement requires expectedText matching the current document range. Read the document or lineInfo first, then retry with expectedText.",
            mapOf("actualLength" to current.length, "actualTextPreview" to current.take(500)),
        )
    }
    if (expected != current) {
        return replacementError(
            "stale_text_range",
            "Replacement range did not match expectedText. Read the current document again and retry with the updated range and expectedText.",
            mapOf(
                "expectedLength" to expected.length,
                "actualLength" to current.length,
                "expectedTextPreview" to expected.take(500),
                "actualTextPreview" to current.take(500),
            ),
        )
    }
    return null
}

private fun JsonObject.stringArgOrNull(name: String): String? {
    val value = get(name)?.takeUnless { it.isJsonNull } ?: return null
    return value.asString
}

private fun replacementError(code: String, summary: String, data: Map<String, Any?>): Map<String, Any?> =
    mapOf(
        "ok" to false,
        "code" to code,
        "summary" to summary,
        "data" to data,
        "affordances" to emptyList<Any>(),
        "limitations" to listOf(mapOf("code" to code, "message" to summary)),
        "nextBestActions" to listOf("text", "lineInfo"),
    )

private fun JsonElement.parseJsonStringValue(): JsonElement {
    if (!isJsonPrimitive || !asJsonPrimitive.isString) return this
    val text = asString.trim()
    if (!text.startsWith("{") && !text.startsWith("[")) return this
    return runCatching { JsonParser.parseString(text) }.getOrDefault(this)
}
