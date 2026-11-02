package com.xique.assistant;

import java.net.URI;

/** Wire validation shared by the bridge and storage. Contains no Android calls. */
final class SessionPolicy {
    static final long MAX_GENERATION = 9007199254740991L;

    static long generation(Object value) {
        if (!(value instanceof Number)) throw new IllegalArgumentException();
        Number number = (Number) value;
        long result = number.longValue();
        if (result < 0 || result >= MAX_GENERATION || number.doubleValue() != (double) result) {
            throw new IllegalArgumentException();
        }
        return result;
    }

    static boolean trustedPage(String value) {
        try {
            URI uri = new URI(value);
            return "https".equals(uri.getScheme()) && "localhost".equals(uri.getHost())
                && uri.getRawUserInfo() == null && uri.getPort() == -1;
        } catch (Exception ignored) { return false; }
    }

    static String origin(Object value) {
        String result = text(value, 1, 2048);
        try {
            URI uri = new URI(result);
            String host = uri.getHost();
            if (!"https".equals(uri.getScheme()) || host == null || host.isEmpty()
                || uri.getRawUserInfo() != null || uri.getRawQuery() != null || uri.getRawFragment() != null
                || (uri.getRawPath() != null && !uri.getRawPath().isEmpty())
                || uri.getPort() == 0 || uri.getPort() > 65535
                || host.equalsIgnoreCase("localhost") || host.equals("[::1]") || host.startsWith("127.")) {
                throw new IllegalArgumentException();
            }
            return result;
        } catch (Exception ignored) { throw new IllegalArgumentException(); }
    }

    static String text(Object value, int min, int max) {
        if (!(value instanceof String)) throw new IllegalArgumentException();
        String result = (String) value;
        int count = result.codePointCount(0, result.length());
        if (count < min || count > max) throw new IllegalArgumentException();
        for (int index = 0; index < result.length(); index++) {
            char c = result.charAt(index);
            if (c < 32 || c == 127) throw new IllegalArgumentException();
        }
        return result;
    }
}
