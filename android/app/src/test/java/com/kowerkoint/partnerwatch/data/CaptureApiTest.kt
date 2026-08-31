package com.kowerkoint.partnerwatch.data

import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.Instant

class CaptureApiTest {
    @Test fun parsesCreatedRequest() {
        val result = CaptureApi().parseCreated(
            """{"requestId":"request","status":"PENDING","createdAt":"2026-08-31T00:00:00Z","expiresAt":"2026-08-31T00:01:00Z"}""",
        )
        assertEquals("request", result.requestId)
        assertEquals(Instant.parse("2026-08-31T00:01:00Z"), result.expiresAt)
    }
}
