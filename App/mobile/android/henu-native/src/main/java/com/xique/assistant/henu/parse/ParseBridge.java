package com.xique.assistant.henu.parse;

import com.xique.assistant.henu.ops.HenuError;

import org.json.JSONObject;

import henuparse.Henuparse;

/**
 * Thin boundary over the gomobile AAR. The Go wrapper's error messages are
 * the fixed safe codes defined on the Go side; they map 1:1 onto the mobile
 * error envelope. The returned JSON string is the design §2 parse DTO and
 * contains no raw HTML or student identity.
 */
public final class ParseBridge {

    private ParseBridge() {
    }

    public static JSONObject parse(byte[] rawBytes) throws HenuError {
        String json;
        try {
            json = Henuparse.parse(rawBytes);
        } catch (Exception e) {
            // gobind surfaces the Go error message.
            String code = String.valueOf(e.getMessage());
            switch (code == null ? "" : code) {
                case "UNSUPPORTED_FILE_TYPE":
                case "INVALID_ENCODING":
                    throw new HenuError("INVALID_ENCODING", "学校响应编码异常");
                case "LOGIN_PAGE":
                    throw new HenuError("UPSTREAM_AUTH_REQUIRED", "学校要求重新登录");
                case "EMPTY_TIMETABLE":
                    // Real empty tables block this milestone; never an empty课表.
                    throw new HenuError("UNRECOGNIZED_DOCUMENT", "学校返回了空课表");
                default:
                    throw new HenuError("UNRECOGNIZED_DOCUMENT", "学校内容无法识别");
            }
        }
        try {
            return new JSONObject(json);
        } catch (Exception e) {
            throw new HenuError("UNRECOGNIZED_DOCUMENT", "解析结果损坏");
        }
    }
}
