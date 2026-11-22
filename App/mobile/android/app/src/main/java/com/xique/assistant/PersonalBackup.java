package com.xique.assistant;

import android.util.JsonReader;
import android.util.JsonToken;

import com.xique.assistant.focus.FocusEngine;
import com.xique.assistant.todo.TodoStore;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.StringReader;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.time.LocalDate;
import java.time.format.DateTimeParseException;
import java.util.HashSet;
import java.util.Locale;
import java.util.Set;

/** A portable file of explicitly selected data, independent of school accounts and preferences. */
final class PersonalBackup {
    static final int MAX_BYTES = 8 * 1024 * 1024;
    static final int MAX_RECORDS = 25_000;
    private static final long MAX_TIMESTAMP = 253_402_300_799_999L;
    private static final String FORMAT = "henubox-personal-backup";

    final long exportedAt;
    final JSONArray todos;
    final JSONArray records;
    final Integer goal;

    private PersonalBackup(long exportedAt, JSONArray todos, JSONArray records, Integer goal) {
        this.exportedAt = exportedAt;
        this.todos = todos;
        this.records = records;
        this.goal = goal;
    }

    static PersonalBackup capture(TodoStore todoStore, FocusEngine focus, Integer goal) throws Exception {
        JSONArray items = todoStore.state().getJSONArray("items");
        JSONArray todos = new JSONArray();
        for (int i = 0; i < items.length(); i++) {
            JSONObject row = items.getJSONObject(i);
            row.remove("revision");
            todos.put(row);
        }
        return new PersonalBackup(System.currentTimeMillis(), todos, focus.backupRecords(MAX_RECORDS), goal);
    }

    byte[] encode() throws JSONException {
        byte[] bytes = new JSONObject().put("format", FORMAT).put("version", 1).put("exported_at", exportedAt)
                .put("todos", todos).put("focus_records", records)
                .put("goal_minutes", goal == null ? JSONObject.NULL : goal)
                .toString().getBytes(StandardCharsets.UTF_8);
        if (bytes.length > MAX_BYTES) throw new IllegalArgumentException("BACKUP_TOO_LARGE");
        return bytes;
    }

    static PersonalBackup read(InputStream input) throws IOException {
        if (input == null) throw new IOException("Cannot open document");
        ByteArrayOutputStream destination = new ByteArrayOutputStream();
        byte[] block = new byte[8192];
        for (int read; (read = input.read(block)) != -1;) {
            if (destination.size() + read > MAX_BYTES) throw new IllegalArgumentException("BACKUP_TOO_LARGE");
            destination.write(block, 0, read);
        }
        try {
            String text = StandardCharsets.UTF_8.newDecoder().decode(ByteBuffer.wrap(destination.toByteArray())).toString();
            try (JsonReader reader = new JsonReader(new StringReader(text))) {
                checkJson(reader, 0);
                if (reader.peek() != JsonToken.END_DOCUMENT) throw invalid();
            }
            JSONObject root = new JSONObject(text);
            if (!FORMAT.equals(root.opt("format"))) throw invalid();
            if (integer(root, "version", 1, Integer.MAX_VALUE) != 1) {
                throw new IllegalArgumentException("UNSUPPORTED_BACKUP_VERSION");
            }
            if (root.length() != 6) throw invalid();
            JSONArray todos = root.getJSONArray("todos");
            JSONArray records = root.getJSONArray("focus_records");
            if (todos.length() > 500 || records.length() > MAX_RECORDS) throw invalid();
            checkRows(todos, true);
            checkRows(records, false);
            long timestamp = integer(root, "exported_at", 1, MAX_TIMESTAMP);
            Integer goal = root.get("goal_minutes") == JSONObject.NULL ? null : (int) integer(root, "goal_minutes", 0, 720);
            return new PersonalBackup(timestamp, todos, records, goal);
        } catch (JSONException | IOException | IllegalStateException | DateTimeParseException e) {
            throw invalid();
        }
    }

    /** Use the strict stream parser to bound nesting and reject duplicate keys before loading JSON rows. */
    private static void checkJson(JsonReader reader, int depth) throws IOException {
        if (depth > 4) throw invalid();
        switch (reader.peek()) {
            case BEGIN_OBJECT:
                reader.beginObject();
                Set<String> keys = new HashSet<>();
                while (reader.hasNext()) {
                    if (!keys.add(reader.nextName())) throw invalid();
                    checkJson(reader, depth + 1);
                }
                reader.endObject();
                break;
            case BEGIN_ARRAY:
                reader.beginArray();
                while (reader.hasNext()) checkJson(reader, depth + 1);
                reader.endArray();
                break;
            case STRING:
            case NUMBER:
                reader.nextString();
                break;
            case BOOLEAN:
                reader.nextBoolean();
                break;
            case NULL:
                reader.nextNull();
                break;
            default:
                throw invalid();
        }
    }

    private static void checkRows(JSONArray rows, boolean todos) throws JSONException {
        Set<String> ids = new HashSet<>();
        for (int i = 0; i < rows.length(); i++) {
            JSONObject row = rows.getJSONObject(i);
            String id = string(row, "id", 36);
            if (!id.matches("[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}")) throw invalid();
            if (!ids.add(id.toLowerCase(Locale.ROOT))) throw invalid();
            // Preserve focus IDs and normalize todo IDs as their respective stores do.
            if (todos) row.put("id", id.toLowerCase(Locale.ROOT));
            if (todos) checkTodo(row);
            else checkFocus(row);
        }
    }

    private static void checkTodo(JSONObject row) throws JSONException {
        if (row.length() != 9) throw invalid();
        String title = string(row, "title", 120);
        if (title.trim().isEmpty() || title.chars().anyMatch(Character::isISOControl)) throw invalid();
        string(row, "note", 2000);
        String date = string(row, "due_date", 10);
        if (!date.isEmpty() && (!date.matches("[0-9]{4}-[0-9]{2}-[0-9]{2}")
                || !LocalDate.parse(date).toString().equals(date) || date.compareTo("2000-01-01") < 0
                || date.compareTo("2100-12-31") > 0)) throw invalid();
        bool(row, "important");
        boolean completed = bool(row, "completed");
        integer(row, "created_at", 0, MAX_TIMESTAMP);
        integer(row, "updated_at", 0, MAX_TIMESTAMP);
        if (completed) integer(row, "completed_at", 0, MAX_TIMESTAMP);
        else if (row.get("completed_at") != JSONObject.NULL) throw invalid();
    }

    private static void checkFocus(JSONObject row) throws JSONException {
        if (row.length() != 7) throw invalid();
        String mode = string(row, "mode", 16);
        if (!mode.matches("focus|countup|short_break|long_break")) throw invalid();
        String label = string(row, "label", 40);
        if (label.chars().anyMatch(Character::isISOControl)) throw invalid();
        integer(row, "started_at", 0, MAX_TIMESTAMP);
        integer(row, "ended_at", 0, MAX_TIMESTAMP);
        integer(row, "actual_ms", 0, 86_400_000);
        if (bool(row, "completed") && !mode.equals("focus")) throw invalid();
    }

    private static String string(JSONObject object, String key, int maximum) throws JSONException {
        Object value = object.get(key);
        if (!(value instanceof String)) throw invalid();
        String text = (String) value;
        if (text.indexOf('\0') >= 0 || text.codePointCount(0, text.length()) > maximum) throw invalid();
        return text;
    }

    private static boolean bool(JSONObject object, String key) throws JSONException {
        Object value = object.get(key);
        if (!(value instanceof Boolean)) throw invalid();
        return (Boolean) value;
    }

    private static long integer(JSONObject object, String key, long minimum, long maximum) throws JSONException {
        Object value = object.get(key);
        if (!(value instanceof Number)) throw invalid();
        Number number = (Number) value;
        long integer = number.longValue();
        if (integer < minimum || integer > maximum || number.doubleValue() != integer) throw invalid();
        return integer;
    }

    private static IllegalArgumentException invalid() {
        return new IllegalArgumentException("INVALID_BACKUP");
    }

    JSONObject summary() throws JSONException {
        return new JSONObject().put("exported_at", exportedAt).put("todos", todos.length())
                .put("focus_records", records.length()).put("goal_minutes", goal == null ? JSONObject.NULL : goal);
    }
}
