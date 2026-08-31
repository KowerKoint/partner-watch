package com.kowerkoint.partnerwatch.capture

import android.app.KeyguardManager
import android.content.Context
import com.kowerkoint.partnerwatch.data.ImageRepository
import com.kowerkoint.partnerwatch.data.UploadedImage
import kotlinx.coroutines.flow.first

class CaptureUploader(
    private val context: Context,
    private val preferences: CapturePreferences,
    private val encoder: JpegEncoder,
    private val images: ImageRepository,
) {
    suspend fun captureAndUpload(): CapturedUpload {
        if (!preferences.accepting.first()) {
            throw CaptureRejectedException
        }
        if (context.getSystemService(KeyguardManager::class.java).isDeviceLocked) {
            throw ScreenshotCaptureException(CaptureFailure.LOCKED)
        }
        val bitmap = PartnerAccessibilityService.captureScreenshot()
        val jpeg = encoder.encode(bitmap)
        return CapturedUpload(images.upload(jpeg), jpeg)
    }
}

data class CapturedUpload(val uploaded: UploadedImage, val jpeg: ByteArray)

data object CaptureRejectedException : Exception("撮影受付が無効です")
