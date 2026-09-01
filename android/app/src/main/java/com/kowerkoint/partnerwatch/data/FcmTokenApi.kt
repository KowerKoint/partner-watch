package com.kowerkoint.partnerwatch.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException

class FcmTokenApi(private val client: OkHttpClient = OkHttpClient()) {
    suspend fun register(session: DeviceSession, token: String) = withContext(Dispatchers.IO) {
        val url = session.serverUrl.resolve("v1/device/fcm-token") ?: throw IOException("トークン登録URLを作成できません")
        val body = JSONObject().put("token", token).toString().toRequestBody(JSON_MEDIA_TYPE)
        val request = Request.Builder().url(url).header("Authorization", "Bearer ${session.credential}").post(body).build()
        client.newCall(request).execute().use { response ->
            if (response.code == 401) throw IOException("端末認証に失敗しました")
            if (!response.isSuccessful) throw IOException("FCMトークンを登録できませんでした")
        }
    }

    private companion object { val JSON_MEDIA_TYPE = "application/json".toMediaType() }
}
