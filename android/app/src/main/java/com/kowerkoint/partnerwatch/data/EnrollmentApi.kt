package com.kowerkoint.partnerwatch.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException

data class EnrollmentResult(
    val deviceId: String,
    val pairId: String,
    val slot: Int,
    val credential: String,
)

sealed class EnrollmentException(message: String) : IOException(message) {
    class InvitationRejected : EnrollmentException("招待コードが無効、期限切れ、または使用済みです")
    class ServerRejected : EnrollmentException("サーバーが登録要求を受け付けませんでした")
    class InvalidResponse : EnrollmentException("サーバーから不正な応答を受信しました")
}

class EnrollmentApi(
    private val client: OkHttpClient = OkHttpClient(),
) {
    suspend fun enroll(
        serverUrl: HttpUrl,
        invitationToken: String,
        deviceName: String,
        publicKey: String,
    ): EnrollmentResult = withContext(Dispatchers.IO) {
        val body = JSONObject()
            .put("invitationToken", invitationToken)
            .put("deviceName", deviceName)
            .put("publicKey", publicKey)
            .toString()
            .toRequestBody(JSON_MEDIA_TYPE)
        val request = Request.Builder()
            .url(serverUrl.resolve("v1/enrollments") ?: throw EnrollmentException.InvalidResponse())
            .post(body)
            .build()

        client.newCall(request).execute().use { response ->
            if (response.code == 404) throw EnrollmentException.InvitationRejected()
            if (!response.isSuccessful) throw EnrollmentException.ServerRejected()
            val responseBody = response.body.string()
            if (responseBody.length > MAX_RESPONSE_CHARS) throw EnrollmentException.InvalidResponse()
            parseResponse(responseBody)
        }
    }

    internal fun parseResponse(value: String): EnrollmentResult = try {
        val json = JSONObject(value)
        val slot = json.getInt("slot")
        EnrollmentResult(
            deviceId = json.getString("deviceId").requireNotBlank(),
            pairId = json.getString("pairId").requireNotBlank(),
            slot = slot.also { require(it == 1 || it == 2) },
            credential = json.getString("credential").requireNotBlank(),
        )
    } catch (_: Exception) {
        throw EnrollmentException.InvalidResponse()
    }

    private fun String.requireNotBlank(): String = also { require(it.isNotBlank()) }

    private companion object {
        val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()
        const val MAX_RESPONSE_CHARS = 16 * 1024
    }
}
