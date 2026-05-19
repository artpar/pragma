package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonObject
import com.google.gson.JsonElement

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

fun JsonObject.jsonArg(name: String): JsonElement? = get(name)?.takeUnless { it.isJsonNull }
