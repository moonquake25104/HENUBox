package com.xique.assistant.focus;

/** Duration arithmetic is independent of wall-clock changes, Android UI and storage. */
public final class FocusClock {
    private FocusClock() {}
    public static long elapsed(long accumulated, long anchor, long now, boolean running) {
        return Math.max(0, accumulated) + (running ? Math.max(0, now-anchor) : 0);
    }
    public static long bounded(long accumulated, long anchor, long now, boolean running, long planned) {
        return Math.min(planned, elapsed(accumulated,anchor,now,running));
    }
    public static String next(String mode, int round, int every) {
        if ("focus".equals(mode)) return round >= every ? "long_break" : "short_break";
        return "focus";
    }
}
