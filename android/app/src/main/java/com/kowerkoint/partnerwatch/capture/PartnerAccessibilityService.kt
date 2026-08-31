package com.kowerkoint.partnerwatch.capture

import android.accessibilityservice.AccessibilityService
import android.graphics.Bitmap
import android.view.Display
import android.view.accessibility.AccessibilityEvent
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.suspendCancellableCoroutine

enum class CaptureFailure {
    LOCKED,
    SERVICE_UNAVAILABLE,
    CAPTURE_PROTECTED,
    RATE_LIMITED,
    INVALID_DISPLAY,
    INTERNAL_ERROR,
}

class ScreenshotCaptureException(val failure: CaptureFailure) : Exception(failure.name)

class PartnerAccessibilityService : AccessibilityService() {
    override fun onServiceConnected() {
        super.onServiceConnected()
        activeService = this
        mutableConnected.value = true
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) = Unit

    override fun onInterrupt() = Unit

    override fun onDestroy() {
        if (activeService === this) {
            activeService = null
            mutableConnected.value = false
        }
        super.onDestroy()
    }

    private suspend fun capture(): Bitmap = suspendCancellableCoroutine { continuation ->
        takeScreenshot(
            Display.DEFAULT_DISPLAY,
            mainExecutor,
            object : TakeScreenshotCallback {
                override fun onSuccess(screenshot: ScreenshotResult) {
                    val hardwareBitmap = Bitmap.wrapHardwareBuffer(
                        screenshot.hardwareBuffer,
                        screenshot.colorSpace,
                    )
                    screenshot.hardwareBuffer.close()
                    if (hardwareBitmap == null) {
                        if (continuation.isActive) {
                            continuation.resumeWith(Result.failure(ScreenshotCaptureException(CaptureFailure.INTERNAL_ERROR)))
                        }
                        return
                    }
                    val softwareBitmap = hardwareBitmap.copy(Bitmap.Config.ARGB_8888, false)
                    hardwareBitmap.recycle()
                    if (!continuation.isActive) {
                        softwareBitmap?.recycle()
                        return
                    }
                    if (softwareBitmap == null) {
                        continuation.resumeWith(Result.failure(ScreenshotCaptureException(CaptureFailure.INTERNAL_ERROR)))
                    } else {
                        continuation.resumeWith(Result.success(softwareBitmap))
                    }
                }

                override fun onFailure(errorCode: Int) {
                    if (!continuation.isActive) return
                    val failure = when (errorCode) {
                        ERROR_TAKE_SCREENSHOT_SECURE_WINDOW -> CaptureFailure.CAPTURE_PROTECTED
                        ERROR_TAKE_SCREENSHOT_INTERVAL_TIME_SHORT -> CaptureFailure.RATE_LIMITED
                        ERROR_TAKE_SCREENSHOT_INVALID_DISPLAY -> CaptureFailure.INVALID_DISPLAY
                        ERROR_TAKE_SCREENSHOT_NO_ACCESSIBILITY_ACCESS -> CaptureFailure.SERVICE_UNAVAILABLE
                        else -> CaptureFailure.INTERNAL_ERROR
                    }
                    continuation.resumeWith(Result.failure(ScreenshotCaptureException(failure)))
                }
            },
        )
    }

    companion object {
        private var activeService: PartnerAccessibilityService? = null
        private val mutableConnected = MutableStateFlow(false)
        val connected: StateFlow<Boolean> = mutableConnected

        suspend fun captureScreenshot(): Bitmap {
            val service = activeService
                ?: throw ScreenshotCaptureException(CaptureFailure.SERVICE_UNAVAILABLE)
            return service.capture()
        }
    }
}
