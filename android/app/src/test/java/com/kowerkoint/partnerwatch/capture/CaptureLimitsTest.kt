package com.kowerkoint.partnerwatch.capture

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CaptureLimitsTest {
    @Test
    fun acceptsSupportedPhoneScreens() {
        assertTrue(CaptureLimits.validDimensions(1080, 2400))
        assertTrue(CaptureLimits.validDimensions(1080, 2340))
    }

    @Test
    fun rejectsInvalidAndOversizedDimensions() {
        assertFalse(CaptureLimits.validDimensions(0, 2400))
        assertFalse(CaptureLimits.validDimensions(2500, 2500))
    }
}
