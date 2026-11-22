package com.xique.assistant;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.SystemClock;

import androidx.activity.result.ActivityResult;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.ActivityCallback;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.xique.assistant.focus.FocusEngine;
import com.xique.assistant.todo.TodoStore;

import org.json.JSONObject;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.time.LocalDate;
import java.time.ZoneId;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

@CapacitorPlugin(name = "XiqueBackup")
public final class XiqueBackupPlugin extends Plugin {
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    // Accessed only on io. At most one small, validated preview is retained.
    private PersonalBackup preview;
    private String previewToken;
    private long previewDeadline;

    @PluginMethod public void exportBackup(PluginCall call) {
        openPicker(call, new Intent(Intent.ACTION_CREATE_DOCUMENT).setType("application/json")
                .putExtra(Intent.EXTRA_TITLE, "HENUBox-备份-" + LocalDate.now(ZoneId.of("Asia/Shanghai")) + ".json"),
                "exportSelected", true);
    }

    @PluginMethod public void previewBackup(PluginCall call) {
        openPicker(call, new Intent(Intent.ACTION_OPEN_DOCUMENT).setType("*/*"), "importSelected", false);
    }

    private void openPicker(PluginCall call, Intent intent, String callback, boolean exporting) {
        getActivity().runOnUiThread(() -> {
            if (!trusted(call)) return;
            try {
                if (exporting) exportGoal(call);
                else BridgeArgs.exact(call);
                startActivityForResult(call, intent.addCategory(Intent.CATEGORY_OPENABLE), callback);
            } catch (IllegalArgumentException e) {
                reject(call, "INVALID_ARGUMENT");
            } catch (Exception e) {
                reject(call, "DOCUMENT_UNAVAILABLE");
            }
        });
    }

    private static Integer exportGoal(PluginCall call) throws Exception {
        long minutes = BridgeArgs.integer(BridgeArgs.exact(call, "goal_minutes"), "goal_minutes");
        if (minutes < -1 || minutes > 720) throw new IllegalArgumentException();
        return minutes == -1 ? null : (int) minutes;
    }

    @ActivityCallback private void exportSelected(PluginCall call, ActivityResult result) {
        withDocument(call, result, uri -> {
            PersonalBackup backup = PersonalBackup.capture(TodoStore.get(getContext()), FocusEngine.get(getContext()), exportGoal(call));
            byte[] bytes = backup.encode();
            try (OutputStream output = getContext().getContentResolver().openOutputStream(uri, "wt")) {
                if (output == null) throw new IOException("Cannot open document");
                output.write(bytes);
            }
            return backup.summary();
        });
    }

    @ActivityCallback private void importSelected(PluginCall call, ActivityResult result) {
        withDocument(call, result, uri -> {
            preview = null;
            try (InputStream input = getContext().getContentResolver().openInputStream(uri)) {
                preview = PersonalBackup.read(input);
            }
            previewToken = UUID.randomUUID().toString();
            previewDeadline = SystemClock.elapsedRealtime() + 10 * 60_000;
            return preview.summary().put("token", previewToken);
        });
    }

    @PluginMethod public void restoreBackup(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!trusted(call)) return;
            execute(call, () -> {
                String token = BridgeArgs.uuid(BridgeArgs.exact(call, "token"), "token");
                if (preview == null || !token.equals(previewToken)
                        || SystemClock.elapsedRealtime() > previewDeadline) {
                    throw new IllegalArgumentException("BACKUP_PREVIEW_EXPIRED");
                }
                // Each store commits one transaction. A retry after a storage failure
                // safely skips any rows whose IDs were already committed.
                int todos = TodoStore.get(getContext()).mergeBackup(preview.todos);
                int records = FocusEngine.get(getContext()).mergeBackup(preview.records);
                JSONObject result = preview.summary().put("added_todos", todos).put("added_focus_records", records);
                preview = null;
                return result;
            });
        });
    }

    @PluginMethod public void discardPreview(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!trusted(call)) return;
            execute(call, () -> {
                String token = BridgeArgs.uuid(BridgeArgs.exact(call, "token"), "token");
                if (token.equals(previewToken)) preview = null;
                return new JSONObject();
            });
        });
    }

    private interface Operation { JSONObject run() throws Exception; }
    private interface DocumentOperation { JSONObject run(Uri uri) throws Exception; }

    private void withDocument(PluginCall call, ActivityResult result, DocumentOperation work) {
        if (call == null) return;
        getActivity().runOnUiThread(() -> {
            if (!trusted(call)) return;
            if (result.getResultCode() != Activity.RESULT_OK) {
                reject(call, "CANCELLED");
                return;
            }
            Uri uri = result.getData() == null ? null : result.getData().getData();
            if (uri == null || !"content".equals(uri.getScheme())) {
                reject(call, "DOCUMENT_UNAVAILABLE");
                return;
            }
            execute(call, () -> work.run(uri));
        });
    }

    private void execute(PluginCall call, Operation operation) {
        io.execute(() -> {
            try {
                call.resolve(new JSObject(operation.run().toString()));
            } catch (IllegalArgumentException e) {
                String code = e.getMessage();
                reject(call, code != null && code.matches("[A-Z_]+") ? code : "INVALID_ARGUMENT");
            } catch (IOException | SecurityException e) {
                reject(call, "DOCUMENT_UNAVAILABLE");
            } catch (Exception e) {
                reject(call, "BACKUP_STORAGE_ERROR");
            }
        });
    }

    private boolean trusted(PluginCall call) {
        if (SessionPolicy.trustedPage(getBridge().getWebView().getUrl()) && getBridge().getServerUrl() == null) return true;
        reject(call, "UNTRUSTED_ORIGIN");
        return false;
    }

    private static void reject(PluginCall call, String code) {
        call.reject("Personal backup operation did not complete", code);
    }

    @Override protected void handleOnDestroy() {
        io.shutdown();
        super.handleOnDestroy();
    }
}
