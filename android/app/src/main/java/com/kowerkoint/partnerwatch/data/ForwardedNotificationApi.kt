package com.kowerkoint.partnerwatch.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException
import java.time.Instant

data class ForwardedNotification(
    val id: String,
    val sourceDeviceId: String,
    val sourceDeviceName: String,
    val sourcePackage: String,
    val sourceAppName: String,
    val title: String,
    val body: String,
    val postedAt: Instant,
)

class ForwardedNotificationApi(private val client: OkHttpClient = OkHttpClient()) {
    suspend fun create(session: DeviceSession, sourcePackage: String, sourceAppName: String, title: String, body: String, postedAt: Instant) = withContext(Dispatchers.IO) {
        val json = JSONObject().put("sourcePackage", sourcePackage).put("sourceAppName", sourceAppName)
            .put("title", title).put("body", body).put("postedAt", postedAt.toString())
        execute(session, "v1/forwarded-notifications", Request.Builder().post(json.toString().toRequestBody(JSON))).use {
            if (it.code != 201) throw IOException("通知を転送できませんでした (${it.code})")
        }
    }

    suspend fun pending(session: DeviceSession): List<ForwardedNotification> = withContext(Dispatchers.IO) {
        execute(session, "v1/forwarded-notifications/pending", Request.Builder().get()).use { response ->
            if (response.code != 200) throw IOException("転送通知を取得できませんでした")
            val array = JSONObject(response.body.string()).getJSONArray("notifications")
            (0 until array.length()).map { index ->
                val item = array.getJSONObject(index)
                ForwardedNotification(item.getString("id"), item.optString("sourceDeviceId"), item.optString("sourceDeviceName"), item.getString("sourcePackage"), item.getString("sourceAppName"), item.getString("title"), item.getString("body"), Instant.parse(item.getString("postedAt")))
            }
        }
    }

    suspend fun acknowledge(session: DeviceSession, id: String) = withContext(Dispatchers.IO) {
        execute(session, "v1/forwarded-notifications/$id/ack", Request.Builder().post(ByteArray(0).toRequestBody(null))).use {
            if (it.code != 204 && it.code != 404) throw IOException("転送通知を確認済みにできませんでした")
        }
    }

    private fun execute(session: DeviceSession, path: String, builder: Request.Builder) = client.newCall(
        builder.url(session.serverUrl.resolve(path) ?: throw IOException("URLが不正です"))
            .header("Authorization", "Bearer ${session.credential}").build(),
    ).execute()

    private companion object { val JSON = "application/json; charset=utf-8".toMediaType() }
}
