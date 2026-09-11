package com.kowerkoint.partnerwatch.data

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

class NotificationFilterTest {
    private val line = ForwardedNotification("id", "pixel", "Pixel", "jp.naver.line.android", "LINE", "Ａｌｉｃｅ", "hello", Instant.EPOCH)

    @Test fun titleContainsNormalizesWidthAndCase() {
        assertTrue(NotificationFilter(sourcePackage="jp.naver.line.android", titlePattern="alice", titleMatch=TextMatch.CONTAINS).matches(line))
    }

    @Test fun exactAndAllConditionsMustMatch() {
        assertTrue(NotificationFilter(sourceDeviceId="pixel", titlePattern="alice", titleMatch=TextMatch.EXACT).matches(line))
        assertFalse(NotificationFilter(sourceDeviceId="other", titlePattern="alice", titleMatch=TextMatch.EXACT).matches(line))
        assertFalse(NotificationFilter(titlePattern="ali", titleMatch=TextMatch.EXACT).matches(line))
    }
}
