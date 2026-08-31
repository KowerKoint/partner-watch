package com.kowerkoint.partnerwatch.capture

import android.graphics.Bitmap
import java.io.ByteArrayOutputStream
import java.io.IOException

object CaptureLimits {
    const val MAX_PIXELS = 5_000_000
    const val MAX_JPEG_BYTES = 10 * 1024 * 1024
    const val JPEG_QUALITY = 85

    fun validDimensions(width: Int, height: Int): Boolean =
        width > 0 && height > 0 && width <= MAX_PIXELS / height
}

class JpegEncoder {
    fun encode(bitmap: Bitmap): ByteArray {
        try {
            require(CaptureLimits.validDimensions(bitmap.width, bitmap.height)) {
                "スクリーンショットの画素数が上限を超えています"
            }
            val output = ByteArrayOutputStream()
            if (!bitmap.compress(Bitmap.CompressFormat.JPEG, CaptureLimits.JPEG_QUALITY, output)) {
                throw IOException("JPEGへの変換に失敗しました")
            }
            return output.toByteArray().also {
                if (it.isEmpty() || it.size > CaptureLimits.MAX_JPEG_BYTES) {
                    throw IOException("JPEGのサイズが上限を超えています")
                }
            }
        } finally {
            bitmap.recycle()
        }
    }
}
