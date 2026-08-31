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
import java.time.Instant

data class UploadedImage(
    val imageId: String,
    val createdAt: Instant,
    val expiresAt: Instant,
)

sealed class ImageApiException(message: String) : IOException(message) {
    class Unauthorized : ImageApiException("端末の認証に失敗しました")
    class NotFound : ImageApiException("画像が存在しないか、すでに取得済みです")
    class Rejected : ImageApiException("サーバーが画像を受け付けませんでした")
    class InvalidResponse : ImageApiException("サーバーから不正な応答を受信しました")
}

class ImageApi(private val client: OkHttpClient = OkHttpClient()) {
    suspend fun upload(serverUrl: HttpUrl, credential: String, jpeg: ByteArray): UploadedImage =
        withContext(Dispatchers.IO) {
            require(jpeg.isNotEmpty() && jpeg.size <= MAX_IMAGE_BYTES)
            val request = authenticatedRequest(serverUrl, credential, "v1/images")
                .post(jpeg.toRequestBody(JPEG_MEDIA_TYPE))
                .build()
            client.newCall(request).execute().use { response ->
                when (response.code) {
                    401 -> throw ImageApiException.Unauthorized()
                }
                if (!response.isSuccessful) throw ImageApiException.Rejected()
                val body = response.body.string()
                if (body.length > MAX_JSON_CHARS) throw ImageApiException.InvalidResponse()
                parseUploadResponse(body)
            }
        }

    suspend fun download(serverUrl: HttpUrl, credential: String, imageId: String): ByteArray =
        withContext(Dispatchers.IO) {
            require(imageId.isNotBlank() && '/' !in imageId && '\\' !in imageId)
            val request = authenticatedRequest(serverUrl, credential, "v1/images/$imageId").get().build()
            client.newCall(request).execute().use { response ->
                when (response.code) {
                    401 -> throw ImageApiException.Unauthorized()
                    404 -> throw ImageApiException.NotFound()
                }
                if (!response.isSuccessful || response.header("Content-Type") != "image/jpeg") {
                    throw ImageApiException.InvalidResponse()
                }
                val stream = response.body.byteStream()
                val bytes = stream.readNBytes(MAX_IMAGE_BYTES + 1)
                if (bytes.isEmpty() || bytes.size > MAX_IMAGE_BYTES) throw ImageApiException.InvalidResponse()
                bytes
            }
        }

    internal fun parseUploadResponse(value: String): UploadedImage = try {
        val json = JSONObject(value)
        UploadedImage(
            imageId = json.getString("imageId").also { require(it.isNotBlank()) },
            createdAt = Instant.parse(json.getString("createdAt")),
            expiresAt = Instant.parse(json.getString("expiresAt")),
        ).also { require(it.expiresAt.isAfter(it.createdAt)) }
    } catch (_: Exception) {
        throw ImageApiException.InvalidResponse()
    }

    private fun authenticatedRequest(serverUrl: HttpUrl, credential: String, path: String): Request.Builder =
        Request.Builder()
            .url(serverUrl.resolve(path) ?: throw ImageApiException.InvalidResponse())
            .header("Authorization", "Bearer $credential")

    private companion object {
        val JPEG_MEDIA_TYPE = "image/jpeg".toMediaType()
        const val MAX_IMAGE_BYTES = 10 * 1024 * 1024
        const val MAX_JSON_CHARS = 16 * 1024
    }
}
