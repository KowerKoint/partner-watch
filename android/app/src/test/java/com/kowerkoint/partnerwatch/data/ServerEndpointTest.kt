package com.kowerkoint.partnerwatch.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ServerEndpointTest {
    @Test
    fun acceptsHttpsOrigin() {
        assertEquals(
            "https://partner-watch.kowerkoint.com/",
            ServerEndpoint.parse(" https://partner-watch.kowerkoint.com ").toString(),
        )
    }

    @Test
    fun rejectsHttpAndUrlWithSecretsOrExtraComponents() {
        assertNull(ServerEndpoint.parse("http://partner-watch.kowerkoint.com"))
        assertNull(ServerEndpoint.parse("https://user:pass@partner-watch.kowerkoint.com"))
        assertNull(ServerEndpoint.parse("https://partner-watch.kowerkoint.com/path"))
        assertNull(ServerEndpoint.parse("https://partner-watch.kowerkoint.com/?token=secret"))
        assertNull(ServerEndpoint.parse("https://partner-watch.kowerkoint.com/#fragment"))
    }
}
