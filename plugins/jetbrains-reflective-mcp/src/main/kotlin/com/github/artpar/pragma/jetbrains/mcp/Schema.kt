package com.github.artpar.pragma.jetbrains.mcp

fun objectSchema(properties: Map<String, Any> = emptyMap(), required: List<String> = emptyList()): Map<String, Any> {
    val schema = linkedMapOf<String, Any>(
        "type" to "object",
        "additionalProperties" to false,
        "properties" to properties,
    )
    if (required.isNotEmpty()) schema["required"] = required
    return schema
}

fun stringProp(description: String): Map<String, Any> = mapOf("type" to "string", "description" to description)

fun intProp(description: String): Map<String, Any> = mapOf("type" to "integer", "description" to description)

fun boolProp(description: String): Map<String, Any> = mapOf("type" to "boolean", "description" to description)

fun arrayProp(description: String): Map<String, Any> = mapOf("type" to "array", "description" to description)
