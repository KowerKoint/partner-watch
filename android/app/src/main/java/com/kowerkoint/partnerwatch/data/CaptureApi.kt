package com.kowerkoint.partnerwatch.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException

class CaptureApi(private val client: OkHttpClient = OkHttpClient()) {
    suspend fun reportReady(session: DeviceSession, requestId: String, imageId: String) =
        report(session, requestId, "READY", imageId, "")

    suspend fun reportFailure(session: DeviceSession, requestId: String, failure: String) =
        report(session, requestId, "FAILED", "", failure)

    private suspend fun report(
        session: DeviceSession,
        requestId: String,
        status: String,
        imageId: String,
        failure: String,
    ) = withContext(Dispatchers.IO) {
        val json = JSONObject().put("status", status).put("imageId", imageId).put("failure", failure)
        val url = session.serverUrl.resolve("v1/capture-requests/$requestId/result")
            ?: throw IOException("結果URLを作成できません")
        val request = Request.Builder().url(url)
            .header("Authorization", "Bearer ${session.credential}")
            .post(json.toString().toRequestBody(JSON)).build()
        client.newCall(request).execute().use {
            if (it.code != 204) throw IOException("結果報告が拒否されました: ${it.code}")
        }
    }

    private companion object { val JSON = "application/json; charset=utf-8".toMediaType() }
}
