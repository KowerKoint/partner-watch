package com.kowerkoint.partnerwatch.data

import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

object ServerEndpoint {
    fun parse(value: String): HttpUrl? {
        val url = value.trim().toHttpUrlOrNull() ?: return null
        if (url.scheme != "https") return null
        if (url.username.isNotEmpty() || url.password.isNotEmpty()) return null
        if (url.query != null || url.fragment != null) return null
        if (url.encodedPath != "/") return null
        return url
    }
}
