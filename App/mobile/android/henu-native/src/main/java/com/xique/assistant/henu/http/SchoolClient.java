package com.xique.assistant.henu.http;

import org.json.JSONObject;

import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.regex.Pattern;

import javax.net.ssl.HttpsURLConnection;

/**
 * Fixed-endpoint school client. The host is a constant — callers cannot
 * supply URLs, cookies from other domains or internal ids. Connections use
 *HttpsURLConnection with redirects disabled (followed manually, at most 3,
 * re-verifying host/scheme), a 15 s per-request timeout and a streamed 8 MiB
 * body cap. Set-Cookie updates are kept in a per-client jar scoped to
 * xk.henu.edu.cn only.
 */
public final class SchoolClient {

    public static final String HOST = "xk.henu.edu.cn";
    public static final String ORIGIN = "https://" + HOST;
    private static final String PATH_TERM = "/jw/common/showYearTerm.action";
    private static final String PATH_TIMETABLE = "/wsxk/xkjg.ckdgxsxdkchj_data10319.jsp";
    private static final String REFERER = ORIGIN + "/student/xkjg.wdkb.jsp?menucode=S20301";
    private static final String UA = "xique-assistant-android/1";
    private static final int TIMEOUT_MS = 15_000;
    private static final int MAX_REDIRECTS = 3;
    private static final int MAX_BODY = 8 << 20;

    /** Safe error codes; messages are fixed and never embed URLs or bodies. */
    public static final class SchoolException extends Exception {
        public final String code;

        SchoolException(String code, String message) {
            super(message);
            this.code = code;
        }
    }

    /** Verified current term; userCode is the internal id and never leaves this class's results. */
    public static final class Term {
        public final String year;
        public final String term;
        public final String label;
        public final String userCode;
        public final String raw;

        Term(String year, String term, String label, String userCode, String raw) {
            this.year = year;
            this.term = term;
            this.label = label;
            this.userCode = userCode;
            this.raw = raw;
        }
    }

    private final Map<String, String> cookieJar = new LinkedHashMap<>();

    public SchoolClient(Map<String, String> candidateCookies) {
        if (candidateCookies != null) {
            // Only simple name=value pairs collected from our own fixed URLs:
            // no separators, control characters or oversized values.
            for (Map.Entry<String, String> e : candidateCookies.entrySet()) {
                String name = e.getKey();
                String value = e.getValue();
                if (name == null || value == null || name.isEmpty() || name.length() > 256
                        || value.length() > 4096
                        || name.contains("=") || name.contains(";") || name.contains("\r") || name.contains("\n")
                        || value.contains(";") || value.contains("\r") || value.contains("\n")) {
                    continue;
                }
                cookieJar.put(name, value);
            }
        }
    }

    public Map<String, String> currentCookies() { return new LinkedHashMap<>(cookieJar); }

    public Term fetchTerm() throws SchoolException {
        Response r = get(PATH_TERM);
        String bodyText = new String(r.body, StandardCharsets.UTF_8);
        if (looksLikeLoginPage(bodyText)) {
            throw new SchoolException("UPSTREAM_AUTH_REQUIRED", "学校要求重新登录");
        }
        try {
            JSONObject o = new JSONObject(bodyText);
            String xn = optTrim(o, "xn");
            String xq = optTrim(o, "xqM");
            String label = optTrim(o, "xqName");
            String userCode = optTrim(o, "userCode");
            if (xn.isEmpty() || xq.isEmpty() || label.isEmpty() || userCode.isEmpty()) {
                throw new SchoolException("UNRECOGNIZED_DOCUMENT", "学期响应不完整");
            }
            return new Term(xn, xq, label, userCode, bodyText);
        } catch (SchoolException e) {
            throw e;
        } catch (Exception e) {
            throw new SchoolException("UNRECOGNIZED_DOCUMENT", "学期响应无法解析");
        }
    }

    public byte[] fetchTimetable(Term term) throws SchoolException {
        String params = buildParams(term);
        Response r = get(PATH_TIMETABLE + "?params=" + params);
        // Raw bytes go to the parser untouched: no transcoding here.
        String probe = new String(r.body, 0, Math.min(r.body.length, 4096), StandardCharsets.ISO_8859_1);
        if (looksLikeLoginPage(probe)) {
            throw new SchoolException("UPSTREAM_AUTH_REQUIRED", "学校要求重新登录");
        }
        return r.body;
    }

    /** Base64(xn=..&xq=..&xh=..) then full query encoding; values validated strictly. */
    static String buildParams(Term term) throws SchoolException {
        if (!DIGITS.matcher(term.year).matches() || term.year.length() > 4) {
            throw new SchoolException("UNRECOGNIZED_DOCUMENT", "学年取值异常");
        }
        if (!DIGITS.matcher(term.term).matches() || term.term.length() > 2) {
            throw new SchoolException("UNRECOGNIZED_DOCUMENT", "学期取值异常");
        }
        if (!SAFE_CODE.matcher(term.userCode).matches() || term.userCode.length() > 64) {
            throw new SchoolException("UNRECOGNIZED_DOCUMENT", "内部标识取值异常");
        }
        String payload = "xn=" + term.year + "&xq=" + term.term + "&xh=" + term.userCode;
        String b64 = Base64.getEncoder().encodeToString(payload.getBytes(StandardCharsets.UTF_8));
        return URLEncoder.encode(b64, StandardCharsets.UTF_8);
    }

    private static final Pattern DIGITS = Pattern.compile("\\d+");
    private static final Pattern SAFE_CODE = Pattern.compile("[A-Za-z0-9]+");

    private static String optTrim(JSONObject o, String key) {
        String v = o.optString(key, "");
        return v == null ? "" : v.trim();
    }

    static boolean looksLikeLoginPage(String text) {
        int hits = 0;
        for (String m : new String[]{"统一身份认证", "用户登录", "请输入密码", "验证码", "登录失败", "会话已过期"}) {
            if (text.contains(m)) hits++;
        }
        return hits >= 2 && !text.contains("上课时间地点");
    }

    private static final class Response {
        final byte[] body;
        final int status;

        Response(int status, byte[] body) {
            this.status = status;
            this.body = body;
        }
    }

    private Response get(String pathAndQuery) throws SchoolException {
        String url = ORIGIN + pathAndQuery;
        IOException lastIo = null;
        for (int hop = 0; hop <= MAX_REDIRECTS; hop++) {
            HttpURLConnection conn = null;
            try {
                conn = open(url);
                int status = conn.getResponseCode();
                absorbSetCookies(conn);
                if (status >= 300 && status < 400) {
                    String location = conn.getHeaderField("Location");
                    if (location == null) {
                        throw new SchoolException("UNRECOGNIZED_DOCUMENT", "重定向缺失目标");
                    }
                    URL next = new URL(new URL(url), location);
                    if (!"https".equals(next.getProtocol()) || !HOST.equals(next.getHost())
                            || (next.getPort() != -1 && next.getPort() != 443)) {
                        throw new SchoolException("AUTH_FLOW_UNSUPPORTED",
                                "认证跳转超出受信范围: " + next.getHost());
                    }
                    if (hop == MAX_REDIRECTS) {
                        throw new SchoolException("UPSTREAM_UNAVAILABLE", "重定向次数超限");
                    }
                    url = next.toString();
                    continue;
                }
                if (status == 429 || status >= 500) {
                    throw new SchoolException("UPSTREAM_UNAVAILABLE", "学校暂时不可用(" + status + ")");
                }
                if (status != 200) {
                    // No auth evidence: an odd 403 is not an account failure.
                    throw new SchoolException("UPSTREAM_UNAVAILABLE", "学校响应异常(" + status + ")");
                }
                byte[] body = readCapped(conn.getInputStream());
                return new Response(status, body);
            } catch (SchoolException e) {
                throw e;
            } catch (javax.net.ssl.SSLException e) {
                throw new SchoolException("UPSTREAM_TLS_ERROR", "学校 TLS 连接失败");
            } catch (IOException e) {
                lastIo = e;
                throw new SchoolException("UPSTREAM_UNAVAILABLE", "学校网络不可达");
            } finally {
                if (conn != null) conn.disconnect();
            }
        }
        throw new SchoolException("UPSTREAM_UNAVAILABLE", "重定向次数超限");
    }

    private HttpURLConnection open(String url) throws IOException {
        URL u = new URL(url);
        if (!"https".equals(u.getProtocol()) || !HOST.equals(u.getHost())) {
            throw new IOException("fixed host violated");
        }
        HttpsURLConnection conn = (HttpsURLConnection) u.openConnection();
        conn.setConnectTimeout(TIMEOUT_MS);
        conn.setReadTimeout(TIMEOUT_MS);
        conn.setInstanceFollowRedirects(false);
        conn.setRequestMethod("GET");
        conn.setRequestProperty("User-Agent", UA);
        conn.setRequestProperty("Referer", REFERER);
        conn.setRequestProperty("Accept", "text/html,application/json,text/plain,*/*");
        StringBuilder cookieHeader = new StringBuilder();
        List<String> names = new ArrayList<>(cookieJar.keySet());
        for (String name : names) {
            if (cookieHeader.length() > 0) cookieHeader.append("; ");
            cookieHeader.append(name).append('=').append(cookieJar.get(name));
        }
        if (cookieHeader.length() > 0 && cookieHeader.length() <= 16 * 1024) {
            conn.setRequestProperty("Cookie", cookieHeader.toString());
        }
        return conn;
    }

    private void absorbSetCookies(HttpURLConnection conn) {
        List<String> values = conn.getHeaderFields().get("Set-Cookie");
        if (values == null) return;
        for (String raw : values) {
            if (raw == null) continue;
            String pair = raw.split(";", 2)[0];
            int eq = pair.indexOf('=');
            if (eq <= 0) continue;
            String name = pair.substring(0, eq).trim();
            String value = pair.substring(eq + 1).trim();
            if (!name.isEmpty()) cookieJar.put(name, value);
        }
    }

    private static byte[] readCapped(InputStream in) throws IOException, SchoolException {
        java.io.ByteArrayOutputStream buf = new java.io.ByteArrayOutputStream();
        byte[] chunk = new byte[16 * 1024];
        long total = 0;
        try (InputStream is = in) {
            int n;
            while ((n = is.read(chunk)) > 0) {
                total += n;
                if (total > MAX_BODY) {
                    throw new SchoolException("RESPONSE_TOO_LARGE", "学校响应超出大小限制");
                }
                buf.write(chunk, 0, n);
            }
        }
        return buf.toByteArray();
    }
}
