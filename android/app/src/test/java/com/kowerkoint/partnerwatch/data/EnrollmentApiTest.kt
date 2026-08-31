package com.kowerkoint.partnerwatch.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class EnrollmentApiTest {
    private val api = EnrollmentApi()

    @Test
    fun parsesEnrollmentResponse() {
        val result = api.parseResponse(
            """{"deviceId":"device","pairId":"pair","slot":2,"credential":"secret"}""",
        )

        assertEquals("device", result.deviceId)
        assertEquals("pair", result.pairId)
        assertEquals(2, result.slot)
        assertEquals("secret", result.credential)
    }

    @Test
    fun rejectsMissingFieldsAndInvalidSlot() {
        assertThrows(EnrollmentException.InvalidResponse::class.java) {
            api.parseResponse("""{"deviceId":"device"}""")
        }
        assertThrows(EnrollmentException.InvalidResponse::class.java) {
            api.parseResponse(
                """{"deviceId":"device","pairId":"pair","slot":3,"credential":"secret"}""",
            )
        }
    }
}
