package com.github.artpar.pragma.jetbrains.mcp

import com.google.gson.JsonObject

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
