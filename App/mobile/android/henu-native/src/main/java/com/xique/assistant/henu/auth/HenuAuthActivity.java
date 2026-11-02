package com.xique.assistant.henu.auth;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.Intent;
import android.net.http.SslError;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.Message;
import android.os.Messenger;
import android.os.RemoteException;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.webkit.CookieManager;
import android.webkit.SslErrorHandler;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.webkit.WebResourceResponse;
import android.webkit.WebStorage;

import com.xique.assistant.henu.http.SchoolClient;

import java.util.ArrayList;
import java.util.List;

/**
 * The school login container: a plain Activity in the :henu_auth process.
 * It renders the school CAS page for the user and nothing else — no JS
 * bridges, no Capacitor, no injected scripts, no credential access. On the
 * post-login landing page it collects the Cookie header for the three fixed
 * school URLs and hands them to the main process over the private Messenger;
 * the MAIN process decides whether that candidate session is valid.
 */
public class HenuAuthActivity extends Activity {

    private static final String LOGIN_URL = SchoolClient.ORIGIN + "/cas/login.action";
    private static final String LANDING_PATH = "/frame/homes.action";
    /** Fixed URLs whose Cookie headers become the candidate session. */
    private static final String[] COOKIE_URLS = {
            SchoolClient.ORIGIN + "/jw/common/showYearTerm.action",
            SchoolClient.ORIGIN + "/student/xkjg.wdkb.jsp?menucode=S20301",
            SchoolClient.ORIGIN + "/wsxk/xkjg.ckdgxsxdkchj_data10319.jsp",
    };

    private String opId;
    private String nonce;
    private long deadline;
    private Messenger reply;
    private boolean sent;
    private boolean cleaning;
    private WebView web;
    private final Handler poll = new Handler(Looper.getMainLooper());

    @Override
    @SuppressLint({"SetJavaScriptEnabled", "AddJavascriptInterface"}) // JS enabled for the school page itself; NO interface is added.
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        if (Build.VERSION.SDK_INT < 28) {
            finishWithError("UNSUPPORTED_DEVICE", "系统版本不支持本机登录");
            return;
        }
        Intent intent = getIntent();
        opId = intent == null ? null : intent.getStringExtra(AuthIpc.EXTRA_OP_ID);
        nonce = intent == null ? null : intent.getStringExtra(AuthIpc.EXTRA_NONCE);
        deadline = intent == null ? 0 : intent.getLongExtra(AuthIpc.EXTRA_DEADLINE, 0);
        Messenger m = intent == null ? null : intent.getParcelableExtra(AuthIpc.EXTRA_MESSENGER);
        if (opId == null || nonce == null || m == null) {
            finish(); // forged or malformed launch
            return;
        }
        reply = m;

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        TextView bar = new TextView(this);
        bar.setText("河南大学教务 — " + SchoolClient.HOST);
        bar.setPadding(24, 20, 24, 20);
        bar.setGravity(Gravity.CENTER_VERTICAL);
        bar.setBackgroundColor(0xFF1F3B70);
        bar.setTextColor(0xFFFFFFFF);
        Button close = new Button(this);
        close.setText("关闭");
        close.setOnClickListener(v -> cleanupAndFinish(false));
        LinearLayout barLayout = new LinearLayout(this);
        barLayout.setOrientation(LinearLayout.HORIZONTAL);
        barLayout.setBackgroundColor(0xFF1F3B70);
        barLayout.addView(bar, new LinearLayout.LayoutParams(0,
                LinearLayout.LayoutParams.WRAP_CONTENT, 1f));
        barLayout.addView(close);
        root.addView(barLayout, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));

        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);            // the school page needs its own scripts
        s.setDomStorageEnabled(true);            // school-side storage only
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        s.setAllowFileAccessFromFileURLs(false);
        s.setAllowUniversalAccessFromFileURLs(false);
        s.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        s.setSupportZoom(false);
        s.setDisplayZoomControls(false);
        web.setWebViewClient(new WebViewClient() {
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                return isSchoolHttps(request.getUrl()) ? null : blocked();
            }

            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view, String url) {
                return isSchoolHttps(android.net.Uri.parse(url)) ? null : blocked();
            }

            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return !isSchoolHttps(request.getUrl());
            }

            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                return !isSchoolHttps(android.net.Uri.parse(url));
            }

            @Override
            public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                handler.cancel(); // TLS problems always cancel; no project CA here
                finishWithError("UPSTREAM_TLS_ERROR", "学校安全连接失败");
            }
        });
        root.addView(web, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.MATCH_PARENT, 1f));
        setContentView(root);

        // Also clean remnants of a previous process crash before showing CAS.
        WebStorage.getInstance().deleteAllData();
        web.clearCache(true);
        CookieManager.getInstance().removeAllCookies(removed -> {
            CookieManager.getInstance().flush();
            if (!cleaning && web != null && !isDestroyed()) {
                web.loadUrl(LOGIN_URL);
                poll.postDelayed(this::pollLanding, 1500);
            }
        });
    }

    static boolean isSchoolHttps(android.net.Uri uri) {
        if (uri == null) return false;
        if (!"https".equals(uri.getScheme())) return false;
        if (!SchoolClient.HOST.equals(uri.getHost())) return false;
        if (uri.getUserInfo() != null) return false;
        int port = uri.getPort();
        return port == -1 || port == 443;
    }

    private static WebResourceResponse blocked() {
        return new WebResourceResponse("text/plain", "UTF-8", 403, "Blocked",
            java.util.Collections.emptyMap(), new java.io.ByteArrayInputStream(new byte[0]));
    }

    private void pollLanding() {
        if (isFinishing() || isDestroyed()) return;
        if (deadline > 0 && System.currentTimeMillis() > deadline) {
            finish(); // the main process enforces its own deadline state
            return;
        }
        String url = web.getUrl();
        if (url != null) {
            android.net.Uri uri = android.net.Uri.parse(url);
            if (isSchoolHttps(uri) && LANDING_PATH.equals(uri.getPath())) {
                sendCandidates();
                return;
            }
        }
        poll.postDelayed(this::pollLanding, 1500);
    }

    private void sendCandidates() {
        if (sent) return;
        sent = true;
        CookieManager cm = CookieManager.getInstance();
        cm.flush();
        List<String> values = new ArrayList<>();
        int total = 0;
        for (String fixed : COOKIE_URLS) {
            String header = cm.getCookie(fixed);
            if (header == null) header = "";
            if (header.getBytes(java.nio.charset.StandardCharsets.UTF_8).length > 16 * 1024) header = "";
            total += header.getBytes(java.nio.charset.StandardCharsets.UTF_8).length;
            values.add(header);
        }
        Message msg = Message.obtain(null, AuthIpc.MSG_COOKIES);
        Bundle data = new Bundle();
        data.putString(AuthIpc.KEY_OP_ID, opId);
        data.putString(AuthIpc.KEY_NONCE, nonce);
        data.putStringArray(AuthIpc.KEY_URLS, COOKIE_URLS);
        data.putStringArray(AuthIpc.KEY_COOKIES, values.toArray(new String[0]));
        msg.setData(data);
        boolean delivered = false;
        if (total <= AuthIpc.MAX_FRAME_BYTES) {
            try {
                reply.send(msg);
                delivered = true;
            } catch (RemoteException ignored) {
            }
        }
        cleanupAndFinish(delivered);
    }

    private void finishWithError(String code, String message) {
        if (reply != null && !sent) {
            sent = true;
            Message msg = Message.obtain(null, AuthIpc.MSG_ERROR);
            Bundle data = new Bundle();
            data.putString(AuthIpc.KEY_OP_ID, opId);
            data.putString(AuthIpc.KEY_NONCE, nonce);
            data.putString(AuthIpc.KEY_MESSAGE, code + ":" + message);
            msg.setData(data);
            try {
                reply.send(msg);
            } catch (RemoteException ignored) {
            }
        }
        cleanupAndFinish(false);
    }

    /** Clears every trace of the school session from THIS process, then exits. */
    private void cleanupAndFinish(boolean waitForCookies) {
        if (cleaning) return;
        cleaning = true;
        poll.removeCallbacksAndMessages(null);
        if (!sent && reply != null) {
            sent = true;
            Message msg = Message.obtain(null, AuthIpc.MSG_CLOSED);
            Bundle data = new Bundle(); data.putString(AuthIpc.KEY_OP_ID, opId);
            data.putString(AuthIpc.KEY_NONCE, nonce); msg.setData(data);
            try { reply.send(msg); } catch (RemoteException ignored) { }
        }
        if (web != null) {
            web.stopLoading();
            web.clearHistory();
            web.clearCache(true);
            web.loadUrl("about:blank");
        }
        CookieManager cm = CookieManager.getInstance();
        WebStorage.getInstance().deleteAllData();
        cm.removeAllCookies(value -> { cm.flush(); finishInternal(); });
        if (web != null) {
            web.destroy();
            web = null;
        }
        // removeAllCookies callback arrives asynchronously; also finish after
        // a bound delay so a missing callback cannot strand the window.
        new Handler(Looper.getMainLooper()).postDelayed(this::finishInternal, 1500);
    }

    private void finishInternal() {
        if (!isFinishing()) super.finish();
    }

    @Override public void finish() {
        if (web == null && reply == null) super.finish();
        else cleanupAndFinish(false);
    }

    @Override
    public void onBackPressed() {
        cleanupAndFinish(false);
    }

    @Override
    protected void onDestroy() {
        if (!cleaning && reply != null) cleanupAndFinish(false);
        poll.removeCallbacksAndMessages(null);
        if (web != null) {
            web.destroy();
            web = null;
        }
        super.onDestroy();
    }
}
