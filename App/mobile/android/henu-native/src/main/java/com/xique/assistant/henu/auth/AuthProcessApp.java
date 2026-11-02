package com.xique.assistant.henu.auth;

import android.app.Application;
import android.os.Build;
import android.webkit.WebView;

/**
 * Selects private WebView profiles before initialization in :henu_auth
 * and :library_browser. The first is for school authorization; the second
 * is a plain persistent website browser. Main apps (Capacitor and probe)
 * retain their default profile. No bridge or store is created here.
 */
public final class AuthProcessApp extends Application {

    public static final String AUTH_PROCESS_SUFFIX = ":henu_auth";
    private static final String DATA_SUFFIX = "henu-auth";

    @Override
    public void onCreate() {
        super.onCreate();
        String process = readProcessName();
        if (process != null && process.endsWith(AUTH_PROCESS_SUFFIX)) {
            if (Build.VERSION.SDK_INT >= 28) {
                WebView.setDataDirectorySuffix(DATA_SUFFIX);
            }
            // Deliberately nothing else initializes here: no store, no
            // network, no bridges in the auth process.
        } else if (process != null && process.endsWith(":library_browser")) {
            if (Build.VERSION.SDK_INT >= 28) WebView.setDataDirectorySuffix("library-browser");
            // A plain website browser: persistent library-site storage is
            // separate from both Capacitor and the school's auth WebView.
        } else if (process != null && process.endsWith(":email_browser")) {
            if (Build.VERSION.SDK_INT >= 28) WebView.setDataDirectorySuffix("email-browser");
        } else if (process != null && process.endsWith(":guide_browser")) {
            if (Build.VERSION.SDK_INT >= 28) WebView.setDataDirectorySuffix("guide-browser");
        }
    }

    private String readProcessName() {
        if (Build.VERSION.SDK_INT >= 28) {
            return Application.getProcessName();
        }
        // Below API 28 the direct-connection feature reports
        // UNSUPPORTED_DEVICE; reading /proc is only a defensive fallback.
        try {
            java.io.BufferedReader r = new java.io.BufferedReader(
                    new java.io.FileReader("/proc/self/cmdline"));
            String line = r.readLine();
            r.close();
            return line == null ? null : line.trim();
        } catch (Exception e) {
            return null;
        }
    }
}
