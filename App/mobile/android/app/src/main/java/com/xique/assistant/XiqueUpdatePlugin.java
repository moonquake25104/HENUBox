package com.xique.assistant;

import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.net.Uri;
import android.os.Looper;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.net.Proxy;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.Locale;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.FutureTask;
import java.util.concurrent.TimeUnit;

import javax.net.ssl.HttpsURLConnection;

/**
 * Check the public GitHub Releases API and open the release page. The app
 * never downloads or installs an APK automatically: the user reviews the
 * release in a browser and installs it through Android's own package flow.
 */
@CapacitorPlugin(name = "XiqueUpdate")
public final class XiqueUpdatePlugin extends Plugin {
    private static final String REPOSITORY = "moonquake25104/HENUBox";
    private static final String RELEASES = "https://github.com/" + REPOSITORY + "/releases";
    private static final String LATEST_API = "https://api.github.com/repos/" + REPOSITORY + "/releases/latest";
    private static final int MAX_RESPONSE = 512 * 1024;
    private final ExecutorService io = Executors.newSingleThreadExecutor();

    private static final class Failure extends Exception {
        final String code;
        Failure(String code) { super(code); this.code = code; }
    }

    private boolean trusted() {
        try {
            Callable<Boolean> read = () -> SessionPolicy.trustedPage(getBridge().getWebView().getUrl())
                    && getBridge().getServerUrl() == null;
            if (Looper.myLooper() == Looper.getMainLooper()) return read.call();
            FutureTask<Boolean> task = new FutureTask<>(read);
            getActivity().runOnUiThread(task);
            return task.get(2, TimeUnit.SECONDS);
        } catch (Exception e) {
            return false;
        }
    }

    private String currentVersion() {
        try {
            String value = getContext().getPackageManager()
                    .getPackageInfo(getContext().getPackageName(), 0).versionName;
            if (value != null && !value.isEmpty()) return value;
        } catch (Exception ignored) {
            // Fall through to a safe display value; the update check still works.
        }
        return "0.0.0";
    }

    private static String readBounded(InputStream input) throws Exception {
        try (InputStream in = input; ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[8192];
            int read;
            int total = 0;
            while ((read = in.read(buffer)) != -1) {
                total += read;
                if (total > MAX_RESPONSE) throw new Failure("INVALID_RELEASE");
                out.write(buffer, 0, read);
            }
            return new String(out.toByteArray(), StandardCharsets.UTF_8);
        }
    }

    @PluginMethod
    public void check(PluginCall call) {
        if (!trusted()) {
            call.reject("Untrusted page", "UNTRUSTED_ORIGIN");
            return;
        }
        try {
            BridgeArgs.exact(call);
        } catch (Exception e) {
            call.reject("Invalid arguments", "INVALID_ARGUMENT");
            return;
        }
        final String current = currentVersion();
        io.execute(() -> {
            HttpsURLConnection connection = null;
            try {
                connection = (HttpsURLConnection) new URL(LATEST_API).openConnection(Proxy.NO_PROXY);
                connection.setConnectTimeout(8000);
                connection.setReadTimeout(10000);
                connection.setInstanceFollowRedirects(false);
                connection.setRequestMethod("GET");
                connection.setRequestProperty("Accept", "application/vnd.github+json");
                connection.setRequestProperty("X-GitHub-Api-Version", "2022-11-28");
                connection.setRequestProperty("User-Agent", "HENUBox/" + current);
                int status = connection.getResponseCode();
                if (status == 404) throw new Failure("NO_RELEASE");
                if (status == 403 || status == 429) throw new Failure("RATE_LIMITED");
                if (status != 200) throw new Failure("UPDATE_CHECK_FAILED");
                String type = connection.getContentType();
                if (type == null || !type.toLowerCase(Locale.ROOT).startsWith("application/json")) {
                    throw new Failure("INVALID_RELEASE");
                }
                JSONObject release = new JSONObject(readBounded(connection.getInputStream()));
                if (release.optBoolean("draft") || release.optBoolean("prerelease")) {
                    throw new Failure("NO_RELEASE");
                }
                String tag = release.optString("tag_name", "").trim();
                if (tag.isEmpty()) throw new Failure("INVALID_RELEASE");
                String releaseUrl = release.optString("html_url", "");
                if (!releaseUrl.startsWith(RELEASES + "/")) releaseUrl = RELEASES + "/latest";
                String apkUrl = null;
                JSONArray assets = release.optJSONArray("assets");
                if (assets != null) {
                    for (int i = 0; i < assets.length(); i++) {
                        JSONObject asset = assets.optJSONObject(i);
                        if (asset == null) continue;
                        String name = asset.optString("name", "");
                        String url = asset.optString("browser_download_url", "");
                        if (name.toLowerCase(Locale.ROOT).endsWith(".apk")
                                && url.startsWith(RELEASES + "/download/")) {
                            apkUrl = url;
                            break;
                        }
                    }
                }
                String body = release.optString("body", "");
                if (body.length() > 6000) body = body.substring(0, 6000);
                JSObject result = new JSObject();
                result.put("current_version", current);
                result.put("tag_name", tag);
                result.put("name", release.optString("name", tag));
                result.put("body", body);
                result.put("html_url", releaseUrl);
                result.put("published_at", release.optString("published_at", ""));
                result.put("apk_url", apkUrl == null ? JSONObject.NULL : apkUrl);
                call.resolve(result);
            } catch (Failure failure) {
                call.reject("Update check failed", failure.code);
            } catch (Exception error) {
                call.reject("Update check failed", "NETWORK");
            } finally {
                if (connection != null) connection.disconnect();
            }
        });
    }

    @PluginMethod
    public void openRelease(PluginCall call) {
        if (!trusted()) {
            call.reject("Untrusted page", "UNTRUSTED_ORIGIN");
            return;
        }
        try {
            String url = BridgeArgs.text(BridgeArgs.exact(call, "url"), "url", 2048);
            if (!url.equals(RELEASES) && !url.equals(RELEASES + "/latest")
                    && !url.startsWith(RELEASES + "/")) {
                throw new IllegalArgumentException();
            }
            getActivity().runOnUiThread(() -> {
                try {
                    getActivity().startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url)));
                    call.resolve();
                } catch (ActivityNotFoundException e) {
                    call.reject("Browser unavailable", "BROWSER_UNAVAILABLE");
                }
            });
        } catch (Exception e) {
            call.reject("Invalid release URL", "INVALID_ARGUMENT");
        }
    }

    @Override
    protected void handleOnDestroy() {
        io.shutdownNow();
    }
}
