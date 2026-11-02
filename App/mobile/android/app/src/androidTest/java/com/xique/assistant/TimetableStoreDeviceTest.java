package com.xique.assistant;

import androidx.test.platform.app.InstrumentationRegistry;
import android.content.Context;
import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.Test;
import static org.junit.Assert.*;

/** Synthetic namespace only: production credentials and timetable are never changed. */
public class TimetableStoreDeviceTest {
    @Test public void atomicReopenIsolationAndStaleWrites() throws Exception {
        Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        SessionVault session = new SessionVault(context, "timetable-device-test");
        TimetableStore store = new TimetableStore(context, "timetable-device-test", session);
        store.clear();
        long blank = session.clear();
        long first = session.write(blank, "https://a.invalid", "a", "synthetic-token");
        assertNull(store.read(first));
        JSONObject payload = new JSONObject().put("schema", 1).put("saved_at", "2026-10-25T00:00:00.000Z")
            .put("courses", new JSONArray().put(new JSONObject().put("id", 101)));
        store.write(first, payload);
        assertEquals(101, new TimetableStore(context, "timetable-device-test", session).read(first)
            .getJSONArray("courses").getJSONObject(0).getInt("id"));
        try { store.write(first, new JSONObject().put("schema", 2)); fail(); }
        catch (Exception expected) { /* Invalid replacement must leave the original snapshot intact. */ }
        assertEquals(1, store.read(first).getInt("schema"));
        long other = session.write(first, "https://a.invalid", "b", "synthetic-other");
        assertNull(store.read(other));
        try { store.write(first, payload); fail(); }
        catch (SessionVault.Failure expected) { assertEquals("SESSION_CHANGED", expected.code); }
        long otherServer = session.write(other, "https://b.invalid", "a", "synthetic-other");
        assertNull(store.read(otherServer));
        session.clear();
        try { store.read(otherServer); fail(); }
        catch (SessionVault.Failure expected) { assertEquals("SESSION_CHANGED", expected.code); }
        store.clear();
    }
}
