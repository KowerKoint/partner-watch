package com.kowerkoint.partnerwatch.data

import com.kowerkoint.partnerwatch.security.DeviceSecurity
class ImageRepository(
    private val api: ImageApi,
    private val sessions: DeviceSessionRepository,
) {
    suspend fun upload(jpeg: ByteArray): UploadedImage {
        val session = sessions.load()
        return api.upload(session.serverUrl, session.credential, jpeg)
    }

    suspend fun download(imageId: String): ByteArray {
        val session = sessions.load()
        return api.download(session.serverUrl, session.credential, imageId)
    }

}
