package com.xique.assistant;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.xique.assistant.henu.ops.HenuService;

import org.json.JSONObject;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

@CapacitorPlugin(name = "XiqueLocal")
public final class XiqueLocalPlugin extends Plugin {
    private final ExecutorService serial = Executors.newSingleThreadExecutor();

    @Override protected void handleOnDestroy() { serial.shutdown(); }

    @PluginMethod public void bootstrap(PluginCall call) {
        onTrusted(call, () -> serial.execute(() -> {
            try {
                BridgeArgs.exact(call);
                HenuService service = LocalCore.service(getContext());
                LocalCore.migrateLegacy(getContext(), service);
                JSObject value = new JSObject(service.bootstrap().toString());
                if (LocalCore.migrationWarning != null) value.put("migration_warning",LocalCore.migrationWarning);
                call.resolve(value);
            } catch (Exception error) {
                BridgeArgs.reject(call, error);
            }
        }));
    }

    @PluginMethod public void setCourseColor(PluginCall call) {
        onTrusted(call, () -> serial.execute(() -> {
            try {
                JSObject data = BridgeArgs.exact(call, "workspace_id", "course_id", "color");
                JSONObject result = LocalCore.service(getContext()).setCourseColor(
                        BridgeArgs.text(data, "workspace_id", 128),
                        BridgeArgs.integer(data, "course_id"),
                        BridgeArgs.text(data, "color", 7));
                call.resolve(new JSObject(result.toString()));
            } catch (Exception error) {
                BridgeArgs.reject(call, error);
            }
        }));
    }

    @PluginMethod public void saveCourse(PluginCall call) {
        onTrusted(call, () -> serial.execute(() -> {
            try {
                JSObject data = BridgeArgs.exact(call,"workspace_id","expected_revision","course_id","sync_id","course");
                JSONObject result = LocalCore.service(getContext()).saveCourse(
                    BridgeArgs.uuid(data,"workspace_id"), BridgeArgs.integer(data,"expected_revision"),
                    BridgeArgs.integer(data,"course_id"), BridgeArgs.uuid(data,"sync_id"), BridgeArgs.object(data,"course"));
                CourseReminders.refreshAsync(getContext());
                call.resolve(new JSObject(result.toString()));
            } catch (Exception error) { BridgeArgs.reject(call,error); }
        }));
    }

    @PluginMethod public void deleteCourse(PluginCall call) {
        onTrusted(call, () -> serial.execute(() -> {
            try {
                JSObject data = BridgeArgs.exact(call,"workspace_id","expected_revision","course_id");
                JSONObject result = LocalCore.service(getContext()).deleteCourse(
                    BridgeArgs.uuid(data,"workspace_id"), BridgeArgs.integer(data,"expected_revision"), BridgeArgs.integer(data,"course_id"));
                CourseReminders.refreshAsync(getContext());
                call.resolve(new JSObject(result.toString()));
            } catch (Exception error) { BridgeArgs.reject(call,error); }
        }));
    }

    @PluginMethod public void getSettings(PluginCall call) {
        onTrusted(call, () -> serial.execute(() -> {
            try {
                JSObject data = BridgeArgs.exact(call, "workspace_id", "semester");
                JSONObject result = LocalCore.service(getContext()).getSettings(
                        BridgeArgs.text(data, "workspace_id", 128),
                        BridgeArgs.text(data, "semester", 32));
                call.resolve(new JSObject(result.toString()));
            } catch (Exception error) {
                BridgeArgs.reject(call, error);
            }
        }));
    }

    @PluginMethod public void putSettings(PluginCall call) {
        onTrusted(call, () -> serial.execute(() -> {
            try {
                JSObject data = BridgeArgs.exact(call, "workspace_id", "expected_revision", "settings");
                JSONObject settings = new JSONObject(
                        BridgeArgs.object(data, "settings").toString());
                JSONObject result = LocalCore.service(getContext()).putSettings(
                        BridgeArgs.text(data, "workspace_id", 128),
                        BridgeArgs.integer(data, "expected_revision"),
                        settings);
                CourseReminders.refreshAsync(getContext());
                call.resolve(new JSObject(result.toString()));
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
