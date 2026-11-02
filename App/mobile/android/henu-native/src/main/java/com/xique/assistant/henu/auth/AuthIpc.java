package com.xique.assistant.henu.auth;

/**
 * Messenger IPC contract between the main process service and the school
 * login Activity running in the :henu_auth process. Every message carries the
 * operation id and a one-shot nonce handed over via the launch intent; frames
 * without both are dropped. The main process additionally verifies
 * sendingUid equals its own uid on API 29+.
 */
public final class AuthIpc {

    // main -> auth (inside launch intent extras)
    public static final String EXTRA_OP_ID = "xique.op_id";
    public static final String EXTRA_NONCE = "xique.nonce";
    public static final String EXTRA_MESSENGER = "xique.messenger";
    public static final String EXTRA_DEADLINE = "xique.deadline";

    // auth -> main replies
    public static final int MSG_COOKIES = 1; // candidate Cookie header values
    public static final int MSG_CLOSED = 2;  // user closed the window
    public static final int MSG_ERROR = 3;   // container failed to start etc.

    public static final String KEY_OP_ID = "op_id";
    public static final String KEY_NONCE = "nonce";
    public static final String KEY_URLS = "urls";       // fixed candidate URLs (String[])
    public static final String KEY_COOKIES = "cookies"; // Cookie header per URL (String[])
    public static final String KEY_MESSAGE = "message";

    /** Total size cap for one IPC cookie frame (64 KiB per the design). */
    public static final int MAX_FRAME_BYTES = 64 * 1024;

    private AuthIpc() {
    }
}
