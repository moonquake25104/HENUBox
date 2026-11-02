package com.xique.assistant;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.xique.assistant.henu.classroom.ClassroomService;

import org.json.JSONObject;

import java.time.LocalDate;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Iterator;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * Read-only classroom availability bridge. Network runs on this plugin's own
 * IO executor, independent from the timetable serial queue, so a classroom
 * query can never delay offline bootstrap or the timetable fetch pipeline.
 * Argument validation is as strict as the other local-school bridges; the
 * refresh flag, when present, must be a real boolean and nothing else.
 */
@CapacitorPlugin(name = "XiqueClassroom")
public final class XiqueClassroomPlugin extends Plugin {
    private final ExecutorService io = Executors.newSingleThreadExecutor();

    @Override protected void handleOnDestroy() { io.shutdown(); }

    @PluginMethod public void getBuildings(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id");
            return classroom().getBuildings(BridgeArgs.uuid(data, "workspace_id"));
        });
    }

    @PluginMethod public void getDay(PluginCall call) {
        run(call, () -> {
            JSObject data = strictDayArgs(call);
            return classroom().getDay(
                    BridgeArgs.uuid(data, "workspace_id"),
                    BridgeArgs.text(data, "semester", 64),
                    BridgeArgs.text(data, "date", 10),
                    BridgeArgs.text(data, "campus_id", 8),
                    BridgeArgs.text(data, "building_id", 8),
                    data.optBoolean("refresh", false));
        });
    }

    /** Five required keys plus an optional boolean refresh; extra or null keys rejected. */
    private static JSObject strictDayArgs(PluginCall call) throws Exception {
        JSObject data = call.getData();
        if (data == null) throw new IllegalArgumentException();
        Set<String> required = new HashSet<>(Arrays.asList(
                "workspace_id", "semester", "date", "campus_id", "building_id"));
        Set<String> allowed = new HashSet<>(required);
        allowed.add("refresh");
        if (data.length() < required.size() || data.length() > allowed.size()) {
            throw new IllegalArgumentException();
        }
        Iterator<String> names = data.keys();
        while (names.hasNext()) {
            String key = names.next();
            if (!allowed.contains(key)) throw new IllegalArgumentException();
            Object value = data.get(key);
            if (value == null || value == JSONObject.NULL) throw new IllegalArgumentException();
        }
        for (String key : required) {
            if (!data.has(key)) throw new IllegalArgumentException();
        }
        if (data.has("refresh") && !(data.get("refresh") instanceof Boolean)) {
            throw new IllegalArgumentException();
        }
        // Strict calendar date; the service recomputes term weeks from it.
        String date = data.getString("date");
        if (date == null || !date.matches("[0-9]{4}-[0-9]{2}-[0-9]{2}")) {
            throw new IllegalArgumentException();
        }
        LocalDate.parse(date);
        String campus = data.getString("campus_id");
        String building = data.getString("building_id");
        if (campus == null || building == null
                || !campus.matches("[0-9]{1,8}") || !building.matches("[0-9]{1,8}")) {
            throw new IllegalArgumentException();
        }
        return data;
    }

    private ClassroomService classroom() {
        return LocalCore.classroom(getContext());
    }

    private interface Action {
        JSONObject run() throws Exception;
    }

    private void run(PluginCall call, Action action) {
        onTrusted(call, () -> io.execute(() -> {
            try {
                call.resolve(new JSObject(action.run().toString()));
            } catch (Exception error) {
                BridgeArgs.reject(call, error);
            }
        }));
    }

    private void onTrusted(PluginCall call, Runnable action) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())
                    || getBridge().getServerUrl() != null) {
                call.reject("Untrusted page", "UNTRUSTED_ORIGIN");
                return;
            }
            action.run();
        });
    }
}
