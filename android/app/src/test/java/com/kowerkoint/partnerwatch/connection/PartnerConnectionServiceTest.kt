package com.kowerkoint.partnerwatch.connection

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.Instant

class PartnerConnectionServiceTest {
    @Test
    fun parsesCaptureRequestEvent() {
        val event = parseCaptureRequestedEvent(
            """{"type":"capture.requested","requestId":"request","expiresAt":"2026-08-31T01:00:00Z"}""",
        )
        assertEquals("request", event?.requestId)
        assertEquals(Instant.parse("2026-08-31T01:00:00Z"), event?.expiresAt)
    }

    @Test
    fun ignoresOtherAndMalformedEvents() {
        assertNull(parseCaptureRequestedEvent("not-json"))
        assertNull(parseCaptureRequestedEvent("""{"type":"capture.completed"}"""))
    }

    @Test
    fun parsesCompletedEvent() {
        val event = parseCaptureCompletedEvent(
            """{"type":"capture.completed","requestId":"request","status":"READY","imageId":"image"}""",
        )
        assertEquals("image", event?.imageId)
    }
}
