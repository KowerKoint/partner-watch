package com.kowerkoint.partnerwatch.data

import com.kowerkoint.partnerwatch.security.DeviceSecurity
import okhttp3.HttpUrl

class ImageRepository(
    private val api: ImageApi,
    private val enrollmentStore: EnrollmentStore,
    private val security: DeviceSecurity,
) {
    suspend fun upload(jpeg: ByteArray): UploadedImage {
        val session = session()
        return api.upload(session.serverUrl, session.credential, jpeg)
    }

    suspend fun download(imageId: String): ByteArray {
        val session = session()
        return api.download(session.serverUrl, session.credential, imageId)
    }

    private suspend fun session(): ImageSession {
        val enrollment = enrollmentStore.load() ?: error("端末が登録されていません")
        val encrypted = enrollmentStore.loadEncryptedCredential() ?: error("認証情報がありません")
        val serverUrl = ServerEndpoint.parse(enrollment.serverUrl) ?: error("保存済みサーバーURLが不正です")
        return ImageSession(serverUrl, security.decryptCredential(encrypted))
    }
}

private data class ImageSession(val serverUrl: HttpUrl, val credential: String)
