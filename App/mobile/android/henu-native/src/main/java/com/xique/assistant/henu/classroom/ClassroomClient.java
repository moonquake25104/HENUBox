package com.xique.assistant.henu.classroom;

import com.xique.assistant.henu.ops.HenuError;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import javax.net.ssl.HttpsURLConnection;

/**
 * Read-only KINGOSOFT public-query client. Exactly three whitelisted POST
 * routes exist here (protocol research 2026-10-29): the droplist metadata
 * endpoint and the SB12 whole-building week matrix. Callers cannot supply
 * URLs, combo names, cookies or arbitrary forms; form values are ASCII and
 * built natively from validated catalog ids. Transport is HTTPS with system
 * certificates, redirects disabled, a 15 s per-request timeout, an 8 MiB
 * body cap and a 45 s budget per client instance. The session-expired script
 * is detected before any table parsing; 405/429/5xx are never disguised as
 * login failures.
 */
public final class ClassroomClient {

    public static final String HOST = "xk.henu.edu.cn";
    public static final String ORIGIN = "https://" + HOST;
    static final String PATH_DROPLIST = "/frame/droplist/getDropLists.action";
    static final String PATH_WEEK_MATRIX = "/kbbp/dykb.fjsickkxzcjc_data.jsp";
    static final String REFERER_SB12 = ORIGIN + "/kbbp/dykb.fjsickkxzcjc.html?menucode=SB12";
    private static final String UA = "xique-assistant-android/1";
    private static final int TIMEOUT_MS = 15_000;
    private static final int MAX_BODY = 8 << 20;
    private static final long BUDGET_MS = 45_000;

    /** All error codes are fixed safe strings; messages never embed bodies. */
    private static HenuError unavailable(String message) {
        return new HenuError("UPSTREAM_UNAVAILABLE", message);
    }

    /** Injectable POST transport; returns the raw response body bytes. */
    public interface Transport {
        byte[] post(String url, String referer, String contentType, byte[] form,
                    Map<String, String> cookies, int timeoutMs, int maxBody) throws IOException;
    }

    private final Map<String, String> cookies;
    private final Transport transport;
    private final long deadline;

    public ClassroomClient(Map<String, String> cookies) {
        this(cookies, null);
    }

    public ClassroomClient(Map<String, String> cookies, Transport transport) {
        Map<String, String> safe = new LinkedHashMap<>();
        if (cookies != null) {
            for (Map.Entry<String, String> e : cookies.entrySet()) {
                String name = e.getKey();
                String value = e.getValue();
                if (name == null || value == null || name.isEmpty() || name.length() > 256
                        || value.length() > 4096
                        || name.contains("=") || name.contains(";") || name.contains("\r") || name.contains("\n")
                        || value.contains(";") || value.contains("\r") || value.contains("\n")) {
                    continue;
                }
                safe.put(name, value);
            }
        }
        this.cookies = safe;
        this.transport = transport != null ? transport : new HttpsTransport();
        this.deadline = System.currentTimeMillis() + BUDGET_MS;
    }

    /** One droplist entry; code and name are never empty. */
    public static final class Entry {
        public final String code;
        public final String name;

        Entry(String code, String name) {
            this.code = code;
            this.name = name;
        }
    }

    /** Term/campus/building/room id values are digit strings with possible leading zeros. */
    static void requireId(String value) throws HenuError {
        if (value == null || value.isEmpty() || value.length() > 8) {
            throw HenuError.invalid("目录编码无效");
        }
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            if (c < '0' || c > '9') throw HenuError.invalid("目录编码无效");
        }
    }

    private byte[] postForm(String path, String referer, Map<String, String> form) throws HenuError {
        if (System.currentTimeMillis() >= deadline) {
            throw unavailable("学校查询整体超时");
        }
        StringBuilder body = new StringBuilder();
        try {
            for (Map.Entry<String, String> e : form.entrySet()) {
                if (body.length() > 0) body.append('&');
                body.append(URLEncoder.encode(e.getKey(), StandardCharsets.UTF_8.name()));
                body.append('=');
                // paramValue embeds nested form fields; its '&' must be form-encoded
                // again, which URLEncoder does for the whole value.
                body.append(URLEncoder.encode(e.getValue(), StandardCharsets.UTF_8.name()));
            }
            byte[] raw = transport.post(ORIGIN + path, referer,
                    "application/x-www-form-urlencoded", body.toString().getBytes(StandardCharsets.US_ASCII),
                    cookies, (int) Math.min(TIMEOUT_MS, Math.max(1, deadline - System.currentTimeMillis())), MAX_BODY);
            // The metadata endpoint returns the same HTTP-200 expiry script
            // as the matrix endpoint, before JSON decoding can take place.
            String probe = new String(raw, StandardCharsets.UTF_8);
            if (probe.contains("凭证已失效")) {
                throw new HenuError("UPSTREAM_AUTH_REQUIRED", "学校会话已失效，请重新登录");
            }
            return raw;
        } catch (HenuError e) {
            throw e;
        } catch (javax.net.ssl.SSLException e) {
            throw new HenuError("UPSTREAM_TLS_ERROR", "学校证书校验失败");
        } catch (IOException e) {
            throw unavailable("学校网络不可达");
        } catch (Exception e) {
            throw unavailable("学校请求构造失败");
        }
    }

    private List<Entry> droplist(String comboBoxName, String paramValue) throws HenuError {
        Map<String, String> form = new LinkedHashMap<>();
        form.put("comboBoxName", comboBoxName);
        form.put("paramValue", paramValue == null ? "" : paramValue);
        form.put("isYXB", "0");
        form.put("isCDDW", "0");
        form.put("isXQ", "0");
        form.put("isDJKSLB", "0");
        form.put("isZY", "0");
        byte[] raw = postForm(PATH_DROPLIST, REFERER_SB12, form);
        String text = new String(raw, StandardCharsets.UTF_8);
        try {
            JSONArray array = new JSONArray(text);
            List<Entry> out = new ArrayList<>();
            for (int i = 0; i < array.length(); i++) {
                JSONObject o = array.getJSONObject(i);
                String code = o.optString("code", "").trim();
                String name = o.optString("name", "").trim();
                if (code.isEmpty() || name.isEmpty() || o.length() != 2) {
                    throw new HenuError("UNRECOGNIZED_DOCUMENT", "目录响应字段异常");
                }
                out.add(new Entry(code, name));
            }
            return out;
        } catch (HenuError e) {
            throw e;
        } catch (JSONException e) {
            throw new HenuError("UNRECOGNIZED_DOCUMENT", "目录响应无法解析");
        }
    }

    /** Published terms with timetable schedules, e.g. code "2026,0". */
    public List<Entry> publishedTerms() throws HenuError {
        return droplist("Ms_KBBP_FBXQLLJXAP", "");
    }

    public List<Entry> campuses() throws HenuError {
        return droplist("MsSchoolArea", "");
    }

    public List<Entry> buildingsOfCampus(String campusId) throws HenuError {
        requireId(campusId);
        return droplist("MsSchoolArea_LF", "ssxq=" + campusId);
    }

    public List<Entry> roomsOfBuilding(String campusId, String buildingId) throws HenuError {
        requireId(campusId);
        requireId(buildingId);
        return droplist("MsSchoolArea_LF_JS", "ssxq=" + campusId + "&ssjzw_m=" + buildingId);
    }

    /**
     * Whole-building week matrix. Only campus/building/week vary; every other
     * field is the protocol-verified ASCII form. selJSI stays empty (no
     * per-room queries), lyjc=1 is mandatory, xqskzs is not sent.
     */
    public byte[] fetchWeekMatrix(String campusId, String buildingId, String year,
                                  String termCode, int week) throws HenuError {
        requireId(campusId);
        requireId(buildingId);
        if (!year.matches("[0-9]{4}") || !termCode.matches("[0-9]{1,2}")) {
            throw HenuError.invalid("学期编码无效");
        }
        if (week < 1 || week > 60) {
            throw HenuError.invalid("周次无效");
        }
        Map<String, String> form = new LinkedHashMap<>();
        form.put("xnxq", year + "," + termCode);
        form.put("xn", year);
        form.put("xn1", "");
        form.put("_xq", "");
        form.put("xq_m", termCode);
        form.put("jslx", "");
        form.put("selXQ", campusId);
        form.put("selLF", buildingId);
        form.put("selJSI", "");
        form.put("selJSLX", "");
        form.put("txtZC", String.valueOf(week));
        form.put("rad_jc", "1");
        form.put("lyjc", "1");
        form.put("menucode_current", "SB12");
        return postForm(PATH_WEEK_MATRIX, REFERER_SB12, form);
    }

    /**
     * HTTPS transport with the same discipline as the shipped timetable
     * client: fixed host, system certificates, redirects disabled, streamed
     * size cap. Session-expiry detection is NOT here; decoded parsing owns it.
     */
    private static final class HttpsTransport implements Transport {
        @Override
        public byte[] post(String url, String referer, String contentType, byte[] form,
                           Map<String, String> cookies, int timeoutMs, int maxBody) throws IOException {
            URL u = new URL(url);
            if (!"https".equals(u.getProtocol()) || !HOST.equals(u.getHost())
                    || (u.getPort() != -1 && u.getPort() != 443)) {
                throw new IOException("fixed host violated");
            }
            HttpsURLConnection conn = (HttpsURLConnection) u.openConnection();
            try {
                conn.setConnectTimeout(timeoutMs);
                conn.setReadTimeout(timeoutMs);
                conn.setInstanceFollowRedirects(false);
                conn.setRequestMethod("POST");
                conn.setDoOutput(true);
                conn.setRequestProperty("User-Agent", UA);
                conn.setRequestProperty("Referer", referer);
                conn.setRequestProperty("Content-Type", contentType);
                StringBuilder header = new StringBuilder();
                for (Map.Entry<String, String> e : cookies.entrySet()) {
                    if (header.length() > 0) header.append("; ");
                    header.append(e.getKey()).append('=').append(e.getValue());
                }
                if (header.length() > 0 && header.length() <= 16 * 1024) {
                    conn.setRequestProperty("Cookie", header.toString());
                }
                try (OutputStream out = conn.getOutputStream()) {
                    out.write(form);
                }
                int status = conn.getResponseCode();
                if (status == 429 || status >= 500) {
                    throw new IOException("school temporarily unavailable(" + status + ")");
                }
                if (status >= 300 && status < 400) {
                    // A redirect on the data endpoints is not a login event and
                    // must not be silently followed.
                    throw new IOException("unexpected redirect(" + status + ")");
                }
                if (status != 200) {
                    throw new IOException("school response abnormal(" + status + ")");
                }
                InputStream in = conn.getInputStream();
                java.io.ByteArrayOutputStream buf = new java.io.ByteArrayOutputStream();
                byte[] chunk = new byte[16 * 1024];
                long total = 0;
                try (InputStream is = in) {
                    int n;
                    while ((n = is.read(chunk)) > 0) {
                        total += n;
                        if (total > maxBody) {
                            throw new IOException("response too large");
                        }
                        buf.write(chunk, 0, n);
                    }
                }
                return buf.toByteArray();
            } finally {
                conn.disconnect();
            }
        }
    }
}
