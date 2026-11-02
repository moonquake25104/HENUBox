package com.xique.assistant;

import android.content.Context;

import com.xique.assistant.henu.classroom.ClassroomService;
import com.xique.assistant.henu.ops.HenuService;

import org.json.JSONObject;

/** One HenuService per process; legacy import runs at most once per process. */
final class LocalCore {
    private static HenuService service;
    private static ClassroomService classroom;
    private static boolean legacyChecked;
    static String migrationWarning;

    private LocalCore() {}

    static synchronized HenuService service(Context context) {
        if (service == null) service = new HenuService(context.getApplicationContext());
        return service;
    }

    /** Classroom queries share the timetable storage handle and never block bootstrap. */
    static synchronized ClassroomService classroom(Context context) {
        if (classroom == null) {
            classroom = new ClassroomService(service(context.getApplicationContext()).database());
        }
        return classroom;
    }

    /**
     * Copies the v1 account snapshot into local-v2 exactly once. A missing,
     * unreadable or account-mismatched snapshot never creates an empty v1 file
     * and never blocks offline bootstrap.
     */
    static synchronized void migrateLegacy(Context context, HenuService target) {
        if (legacyChecked) return;
        try {
            if (target.bootstrap().getJSONArray("courses").length() > 0) { legacyChecked = true; return; }
            SessionVault vault = new SessionVault(context);
            JSONObject envelope = vault.read();
            JSONObject session = envelope.optJSONObject("session");
            if (session != null) {
                long generation = envelope.optLong("generation", -1);
                JSONObject snapshot = new TimetableStore(context).read(generation);
                if (snapshot != null) target.importLegacySnapshot(snapshot, generation);
                else if (new java.io.File(context.getNoBackupFilesDir(),"timetable-v1.sqlite").exists())
                    migrationWarning = "旧本机课表未能核实，原文件已保留。";
            }
            legacyChecked = true;
        } catch (Exception ignored) {
            migrationWarning = "旧本机课表暂未能读取，原文件已保留。";
            // Keep the original v1 file untouched; bootstrap still returns the
            // local-v2 courses that already exist.
        }
    }
}
