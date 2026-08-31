package com.kowerkoint.partnerwatch.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.time.Instant

class ImageApiTest {
    private val api = ImageApi()

    @Test
    fun parsesUploadResponse() {
        val result = api.parseUploadResponse(
            """{"imageId":"image","createdAt":"2026-08-31T00:00:00Z","expiresAt":"2026-08-31T01:00:00Z"}""",
        )
        assertEquals("image", result.imageId)
        assertEquals(Instant.parse("2026-08-31T01:00:00Z"), result.expiresAt)
    }

    @Test
    fun rejectsInvalidUploadResponse() {
        assertThrows(ImageApiException.InvalidResponse::class.java) {
            api.parseUploadResponse(
                """{"imageId":"image","createdAt":"2026-08-31T01:00:00Z","expiresAt":"2026-08-31T00:00:00Z"}""",
            )
        }
    }
}
