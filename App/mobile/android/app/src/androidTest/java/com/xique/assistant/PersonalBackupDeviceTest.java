package com.xique.assistant;

import android.content.Context;
import android.content.ContextWrapper;
import android.database.sqlite.SQLiteDatabase;

import androidx.test.platform.app.InstrumentationRegistry;

import com.xique.assistant.focus.FocusEngine;
import com.xique.assistant.todo.TodoStore;

import org.json.JSONObject;
import org.junit.Test;

import java.io.ByteArrayInputStream;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.util.UUID;

import static org.junit.Assert.*;
import static org.junit.Assume.assumeTrue;

public class PersonalBackupDeviceTest {
    private static final String TITLE = "合成待办";
    private static final String LABEL = "合成专注";
    private final Context app = InstrumentationRegistry.getInstrumentation().getTargetContext();

    @Test public void roundTripPreservesCompletionAndMergesIdempotently() throws Exception {
        File source = new File(app.getCacheDir(), "backup-source-" + UUID.randomUUID());
        File target = new File(app.getCacheDir(), "backup-target-" + UUID.randomUUID());
        assertTrue(source.mkdir());
        assertTrue(target.mkdir());
        try (TodoStore original = new TodoStore(isolate(source)); TodoStore restored = new TodoStore(isolate(target))) {
            TestClock sourceClock = new TestClock();
            FocusEngine originalFocus = new FocusEngine(isolate(source), sourceClock);
            FocusEngine restoredFocus = new FocusEngine(isolate(target), new TestClock());
            try {
                seed(original, originalFocus, sourceClock);
                PersonalBackup exported = PersonalBackup.capture(original, originalFocus, 45);
                PersonalBackup file = PersonalBackup.read(new ByteArrayInputStream(exported.encode()));
                assertEquals(45, file.goal.intValue());
                assertEquals(0, restored.state().getJSONArray("items").length());
                assertEquals(0, restoredFocus.getState().getJSONArray("records").length());
                assertEquals(1, restored.mergeBackup(file.todos));
                assertEquals(1, restoredFocus.mergeBackup(file.records));
                JSONObject todo = restored.state().getJSONArray("items").getJSONObject(0);
                assertEquals(TITLE, todo.getString("title"));
                assertEquals("保留备注\n第二行", todo.getString("note"));
                assertTrue(todo.getBoolean("completed"));
                assertEquals(file.todos.getJSONObject(0).getLong("completed_at"), todo.getLong("completed_at"));
                JSONObject focus = restoredFocus.getState().getJSONArray("records").getJSONObject(0);
                assertEquals(LABEL, focus.getString("label"));
                assertEquals(1500_000, focus.getLong("actual_ms"));
                assertTrue(focus.getBoolean("completed"));
                restored.update(todo.getString("id"), todo.getLong("revision"), "本机已修改", "", "", false);
                assertEquals(0, restored.mergeBackup(file.todos));
                assertEquals(0, restoredFocus.mergeBackup(file.records));
                assertEquals("本机已修改", restored.state().getJSONArray("items").getJSONObject(0).getString("title"));

                JSONObject forged = new JSONObject(new String(exported.encode(), StandardCharsets.UTF_8));
                forged.getJSONArray("focus_records").getJSONObject(0).put("mode", "unknown");
                assertRejected(forged.toString(), "INVALID_BACKUP");
                forged.put("version", 2);
                assertRejected(forged.toString(), "UNSUPPORTED_BACKUP_VERSION");
                assertRejected("{\"format\":\"henubox-personal-backup\",\"format\":\"duplicate\"}", "INVALID_BACKUP");
            } finally {
                originalFocus.close();
                restoredFocus.close();
            }
        } finally {
            for (File directory : new File[]{source, target}) {
                SQLiteDatabase.deleteDatabase(new File(directory, "todos.db"));
                SQLiteDatabase.deleteDatabase(new File(directory, "focus.db"));
                assertTrue(directory.delete());
            }
        }
    }

    @Test public void seedEmptyEmulatorForFilePicker() throws Exception {
        assumeTrue("Only run against the explicitly selected, empty, read-only emulator overlay",
                "true".equals(InstrumentationRegistry.getArguments().getString("backup_ui_fixture")));
        TodoStore store = TodoStore.get(app);
        TestClock clock = new TestClock();
        FocusEngine engine = new FocusEngine(app, clock);
        try {
            assertEquals(0, store.state().getJSONArray("items").length());
            assertEquals(0, engine.getState().getJSONArray("records").length());
            seed(store, engine, clock);
        } finally {
            engine.close();
        }
    }

    @Test public void verifyFilePickerRestoredRecords() throws Exception {
        assumeTrue("Only run after the isolated emulator has restored the sample document",
                "true".equals(InstrumentationRegistry.getArguments().getString("backup_ui_restore")));
        var items = TodoStore.get(app).state().getJSONArray("items");
        var records = FocusEngine.get(app).getState().getJSONArray("records");
        assertEquals(1, items.length());
        assertEquals(TITLE, items.getJSONObject(0).getString("title"));
        assertTrue(items.getJSONObject(0).getBoolean("completed"));
        assertEquals(1, records.length());
        assertEquals(LABEL, records.getJSONObject(0).getString("label"));
        assertEquals(1500_000, records.getJSONObject(0).getLong("actual_ms"));
    }

    private Context isolate(File directory) {
        return new ContextWrapper(app) {
            @Override public File getDatabasePath(String name) { return new File(directory, new File(name).getName()); }
        };
    }

    private static class TestClock implements FocusEngine.Clock {
        long elapsed;
        long wall = System.currentTimeMillis() - 1500_000;
        public long elapsed() { return elapsed; }
        public long wall() { return wall + elapsed; }
        public int boot() { return 1; }
    }

    private static void seed(TodoStore todos, FocusEngine focus, TestClock clock) throws Exception {
        String id = UUID.randomUUID().toString();
        JSONObject todo = todos.create(id, TITLE, "保留备注\n第二行", "", true).getJSONArray("items").getJSONObject(0);
        todos.complete(id, todo.getLong("revision"), true);
        focus.command("start", 0, UUID.randomUUID().toString(), "focus", LABEL);
        clock.elapsed = 1500_000;
        focus.getState();
    }

    private static void assertRejected(String text, String code) throws Exception {
        try {
            PersonalBackup.read(new ByteArrayInputStream(text.getBytes(StandardCharsets.UTF_8)));
            fail("Expected backup validation to reject the document");
        } catch (IllegalArgumentException expected) {
            assertEquals(code, expected.getMessage());
        }
    }
}
