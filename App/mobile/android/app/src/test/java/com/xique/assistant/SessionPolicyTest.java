package com.xique.assistant;

import org.junit.Test;
import static org.junit.Assert.*;

public class SessionPolicyTest {
    @Test public void originRejectsCredentialsPathsLoopbackAndNonTls() {
        assertEquals("https://192.168.137.1:8443", SessionPolicy.origin("https://192.168.137.1:8443"));
        for (Object bad : new Object[]{null, 1, "http://example.com", "https://a@b", "https://b/", "https://b?q=1", "https://b#x", "https://localhost", "https://127.0.0.1", "https://[::1]", "https://b:0"}) {
            try { SessionPolicy.origin(bad); fail("accepted invalid origin"); } catch (IllegalArgumentException expected) { }
        }
    }
    @Test public void generationRejectsCoercionAndUnsafeNumbers() {
        assertEquals(3, SessionPolicy.generation(3L));
        for (Object bad : new Object[]{null, "3", -1, 0.5, Double.NaN, Double.POSITIVE_INFINITY, 9007199254740992L}) {
            try { SessionPolicy.generation(bad); fail("accepted invalid generation"); } catch (IllegalArgumentException expected) { }
        }
    }
    @Test public void onlyPackagedOriginCanUseBridge() {
        assertTrue(SessionPolicy.trustedPage("https://localhost/settings"));
        for (String bad : new String[]{"https://localhost.evil", "http://localhost", "https://evil@localhost", "https://localhost:443", "file:///a", "https://other"}) {
            assertFalse(SessionPolicy.trustedPage(bad));
        }
    }
}
