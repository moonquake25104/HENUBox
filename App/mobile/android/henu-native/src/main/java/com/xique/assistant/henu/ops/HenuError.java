package com.xique.assistant.henu.ops;

/**
 * Fixed safe error envelope for every native service method. The message is
 * a constant Chinese string; raw exceptions, URLs, cookies and HTML never
 * travel in it.
 */
public final class HenuError extends Exception {
    public final String code;

    public HenuError(String code, String message) {
        super(message);
        this.code = code;
    }

    public static HenuError invalid(String message) {
        return new HenuError("INVALID_ARGUMENT", message);
    }

    public static HenuError notFound(String message) {
        return new HenuError("NOT_FOUND", message);
    }

    public static HenuError storage(String message) {
        return new HenuError("STORAGE_UNAVAILABLE", message);
    }
}
