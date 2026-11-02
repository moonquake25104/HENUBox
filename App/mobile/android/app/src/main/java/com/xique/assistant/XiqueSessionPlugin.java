package com.xique.assistant;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

@CapacitorPlugin(name = "XiqueSession")
public final class XiqueSessionPlugin extends Plugin {
    private final ExecutorService serial = Executors.newSingleThreadExecutor();
    private SessionVault vault;

    @Override public void load() { vault = new SessionVault(getContext()); }
    @Override protected void handleOnDestroy() { serial.shutdown(); }

    @PluginMethod public void read(PluginCall call) { dispatch(call, "read"); }
    @PluginMethod public void write(PluginCall call) { dispatch(call, "write"); }
    @PluginMethod public void clear(PluginCall call) { dispatch(call, "clear"); }

    private void dispatch(PluginCall call, String action) {
        // WebView URL must be read on its UI thread. Capacitor's modern bridge
        // additionally restricts messages to the main frame and configured local origin.
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl())
                || getBridge().getServerUrl() != null) {
                call.reject("Untrusted page", "UNTRUSTED_ORIGIN");
                return;
            }
            serial.execute(() -> {
                try {
                    if (action.equals("read")) {
                        call.resolve(new JSObject(vault.read().toString()));
                        return;
                    }
                    long generation;
                    if (action.equals("clear")) generation = vault.clear();
                    else {
                        JSObject data = call.getData();
                        if (data.length() != 4) throw new IllegalArgumentException();
                        generation = vault.write(SessionPolicy.generation(data.get("expected_generation")),
                            SessionPolicy.origin(data.get("server_origin")),
                            SessionPolicy.text(data.get("account_id"), 1, 256),
                            SessionPolicy.text(data.get("access_token"), 1, 32768));
                    }
                    call.resolve(new JSObject().put("generation", generation));
                } catch (IllegalArgumentException | org.json.JSONException e) {
                    call.reject("Invalid session arguments", "INVALID_ARGUMENT");
                } catch (SessionVault.Failure e) {
                    call.reject("Session storage unavailable", e.code);
                } catch (Exception e) {
                    call.reject("Session storage unavailable", "STORAGE_FAILURE");
                }
            });
        });
    }
}
