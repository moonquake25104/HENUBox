package com.xique.assistant;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.xique.assistant.henu.ops.HenuService;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

@CapacitorPlugin(name = "XiqueHenu")
public final class XiqueHenuPlugin extends Plugin {
    private final ExecutorService serial = Executors.newSingleThreadExecutor();

    @Override protected void handleOnDestroy() { serial.shutdown(); }

    @PluginMethod public void getConnection(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id");
            return service().getConnection(BridgeArgs.text(data, "workspace_id", 128));
        });
    }

    @PluginMethod public void startLogin(PluginCall call) {
        // Activity-result registration must stay on the UI thread; the service
        // itself validates idempotency and only schedules work.
        onTrusted(call, () -> getActivity().runOnUiThread(() -> {
            try {
                JSObject data = BridgeArgs.exact(call, "workspace_id", "idempotency_key");
                JSONObject result = service().startLogin(
                        BridgeArgs.text(data, "workspace_id", 128),
                        BridgeArgs.text(data, "idempotency_key", 64),
                        getActivity());
                call.resolve(new JSObject(result.toString()));
            } catch (Exception error) {
                BridgeArgs.reject(call, error);
            }
        }));
    }

    @PluginMethod public void verifyLogin(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id", "operation_id");
            return service().verifyLogin(BridgeArgs.text(data, "workspace_id", 128),
                    BridgeArgs.text(data, "operation_id", 64));
        });
    }

    @PluginMethod public void getOperation(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id", "operation_id");
            return service().getOperation(BridgeArgs.text(data, "workspace_id", 128),
                    BridgeArgs.text(data, "operation_id", 64));
        });
    }

    @PluginMethod public void cancelOperation(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id", "operation_id");
            return service().cancelOperation(
                    BridgeArgs.text(data, "workspace_id", 128),
                    BridgeArgs.text(data, "operation_id", 64));
        });
    }

    @PluginMethod public void fetchPreview(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id", "semester",
                    "expected_revision", "idempotency_key");
            return service().fetchPreview(
                    BridgeArgs.text(data, "workspace_id", 128),
                    BridgeArgs.text(data, "semester", 32),
                    BridgeArgs.integer(data, "expected_revision"),
                    BridgeArgs.text(data, "idempotency_key", 64));
        });
    }

    @PluginMethod public void getPreview(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id", "preview_id");
            return service().getPreview(BridgeArgs.text(data, "workspace_id", 128),
                    BridgeArgs.text(data, "preview_id", 64));
        });
    }

    @PluginMethod public void commitPreview(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id", "preview_id",
                    "expected_revision", "idempotency_key", "acknowledged_warning_codes");
            JSONArray warnings = BridgeArgs.stringArray(data, "acknowledged_warning_codes");
            JSONObject committed = service().commitPreview(
                    BridgeArgs.text(data, "workspace_id", 128),
                    BridgeArgs.text(data, "preview_id", 64),
                    BridgeArgs.integer(data, "expected_revision"),
                    BridgeArgs.text(data, "idempotency_key", 64),
                    warnings);
            CourseReminders.refreshAsync(getContext());
            return committed;
        });
    }

    @PluginMethod public void disconnect(PluginCall call) {
        run(call, () -> {
            JSObject data = BridgeArgs.exact(call, "workspace_id",
                    "expected_connection_generation");
            return service().disconnect(BridgeArgs.text(data, "workspace_id", 128),
                    BridgeArgs.integer(data, "expected_connection_generation"));
        });
    }

    private HenuService service() {
        return LocalCore.service(getContext());
    }

    private interface Action {
        JSONObject run() throws Exception;
    }

    private void run(PluginCall call, Action action) {
        onTrusted(call, () -> serial.execute(() -> {
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
