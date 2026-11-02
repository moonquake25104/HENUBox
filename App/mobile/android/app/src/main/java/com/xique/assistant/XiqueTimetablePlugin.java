package com.xique.assistant;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONObject;

@CapacitorPlugin(name = "XiqueTimetable")
public final class XiqueTimetablePlugin extends Plugin {
    private final ExecutorService serial = Executors.newSingleThreadExecutor();
    private TimetableStore store;
    @Override public void load() { store = new TimetableStore(getContext()); }
    @Override protected void handleOnDestroy() { serial.shutdown(); }
    @PluginMethod public void read(PluginCall call) { dispatch(call, false); }
    @PluginMethod public void write(PluginCall call) { dispatch(call, true); }

    private void dispatch(PluginCall call, boolean write) {
        getActivity().runOnUiThread(() -> {
            if (!SessionPolicy.trustedPage(getBridge().getWebView().getUrl()) || getBridge().getServerUrl() != null) {
                call.reject("Untrusted page", "UNTRUSTED_ORIGIN"); return;
            }
            serial.execute(() -> {
                try {
                    JSObject data = call.getData();
                    if (data.length() != (write ? 2 : 1)) throw new IllegalArgumentException();
                    long generation = SessionPolicy.generation(data.get("expected_generation"));
                    if (write) {
                        store.write(generation, data.getJSONObject("snapshot"));
                        call.resolve(new JSObject().put("saved", true));
                    } else {
                        JSONObject value = store.read(generation);
                        call.resolve(new JSObject().put("snapshot", value == null ? JSONObject.NULL : value));
                    }
                } catch (SessionVault.Failure e) {
                    call.reject("Session changed", e.code);
                } catch (IllegalArgumentException | org.json.JSONException e) {
                    call.reject("Invalid timetable arguments", "INVALID_ARGUMENT");
                } catch (Exception e) {
                    call.reject("Local timetable storage unavailable", "STORAGE_FAILURE");
                }
            });
        });
    }
}
