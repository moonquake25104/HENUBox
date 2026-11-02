package com.xique.assistant.henu.classroom;

import com.xique.assistant.henu.ops.HenuError;

import org.json.JSONObject;

import henuparse.Henuparse;

/**
 * Thin boundary over the gomobile AAR for the classroom week matrix. The Go
 * wrapper's error messages are the fixed safe codes defined on the Go side;
 * they map onto the mobile error envelope decided for the classroom bridge.
 */
public final class ClassroomParse {

    private ClassroomParse() {
    }

    public static JSONObject parseWeekMatrix(byte[] raw) throws HenuError {
        String json;
        try {
            json = Henuparse.parseClassroomWeek(raw);
        } catch (Exception e) {
            String code = String.valueOf(e.getMessage());
            switch (code == null ? "" : code) {
                case "INVALID_ENCODING":
                    throw new HenuError("UNRECOGNIZED_DOCUMENT", "学校响应编码异常");
                case "LOGIN_PAGE":
                    throw new HenuError("UPSTREAM_AUTH_REQUIRED", "学校要求重新登录");
                default:
                    throw new HenuError("UNRECOGNIZED_DOCUMENT", "教室内容无法识别");
            }
        }
        try {
            return new JSONObject(json);
        } catch (Exception e) {
            throw new HenuError("UNRECOGNIZED_DOCUMENT", "解析结果损坏");
        }
    }
}
