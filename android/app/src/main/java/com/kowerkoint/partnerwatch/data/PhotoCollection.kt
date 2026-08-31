package com.kowerkoint.partnerwatch.data

import android.content.ContentValues
import android.content.Context
import android.net.Uri
import android.provider.MediaStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.IOException
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

class PhotoCollection(private val context: Context) {
    suspend fun save(jpeg: ByteArray, capturedAt: Instant = Instant.now()): Uri = withContext(Dispatchers.IO) {
        val displayName = "PartnerWatch_${FILE_TIME.format(capturedAt.atZone(ZoneId.systemDefault()))}.jpg"
        val values = ContentValues().apply {
            put(MediaStore.Images.Media.DISPLAY_NAME, displayName)
            put(MediaStore.Images.Media.MIME_TYPE, "image/jpeg")
            put(MediaStore.Images.Media.RELATIVE_PATH, "Pictures/Partner Watch")
            put(MediaStore.Images.Media.IS_PENDING, 1)
        }
        val resolver = context.contentResolver
        val uri = resolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values)
            ?: throw IOException("写真コレクションを作成できませんでした")
        try {
            resolver.openOutputStream(uri, "w")?.use { it.write(jpeg) }
                ?: throw IOException("写真を書き込めませんでした")
            values.clear()
            values.put(MediaStore.Images.Media.IS_PENDING, 0)
            if (resolver.update(uri, values, null, null) != 1) {
                throw IOException("写真を公開できませんでした")
            }
            uri
        } catch (error: Exception) {
            resolver.delete(uri, null, null)
            throw error
        }
    }

    private companion object {
        val FILE_TIME: DateTimeFormatter = DateTimeFormatter.ofPattern("yyyyMMdd_HHmmss")
    }
}
