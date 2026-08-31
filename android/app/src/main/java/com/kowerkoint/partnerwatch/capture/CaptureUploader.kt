package com.kowerkoint.partnerwatch.capture

import com.kowerkoint.partnerwatch.data.ImageRepository
import com.kowerkoint.partnerwatch.data.UploadedImage
import kotlinx.coroutines.flow.first

class CaptureUploader(
    private val preferences: CapturePreferences,
    private val encoder: JpegEncoder,
    private val images: ImageRepository,
) {
    suspend fun captureAndUpload(): UploadedImage {
        if (!preferences.accepting.first()) {
            throw CaptureRejectedException
        }
        val bitmap = PartnerAccessibilityService.captureScreenshot()
        return images.upload(encoder.encode(bitmap))
    }
}

data object CaptureRejectedException : Exception("撮影受付が無効です")
