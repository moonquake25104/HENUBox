package com.xique.assistant;

import android.annotation.SuppressLint;
import android.graphics.Color;
import android.graphics.Typeface;
import android.net.Uri;
import android.net.http.SslError;
import android.os.Bundle;
import android.os.Build;
import android.text.TextUtils;
import android.view.Gravity;
import android.view.View;
import android.webkit.CookieManager;
import android.webkit.SslErrorHandler;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.TextView;
import android.widget.Toast;
import androidx.activity.OnBackPressedCallback;
import androidx.appcompat.app.AppCompatActivity;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsCompat;
import androidx.core.view.WindowInsetsControllerCompat;

/** Plain website browser in :library_browser. No Capacitor, JS interfaces, scraping or automation. */
public class LibraryBrowserActivity extends AppCompatActivity {
    static final String HOME = "https://zwyy.henu.edu.cn/h5/index.html#/mo/home";
    private WebView web;
    private TextView host;
    private ProgressBar progress;
    private LinearLayout errorPanel;
    private TextView errorText;
    private String retryUrl;
    private View fullScreen;
    private WebChromeClient.CustomViewCallback fullScreenCallback;
    protected String entryUrl() { return HOME; }
    protected String browserTitle() { return "图书馆预约"; }
    protected String closeDescription() { return "关闭图书馆网页"; }
    protected boolean guideExtras() { return false; }

    @Override @SuppressLint("SetJavaScriptEnabled")
    @SuppressWarnings("deprecation")
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        retryUrl = entryUrl();
        boolean dark = getSharedPreferences("xique-appearance", MODE_PRIVATE)
                .getString("theme", "light").equals("dark");
        String style=getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("style","classic"),palette=getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("palette","original");
        int background = guideExtras() ? Color.parseColor(style.equals("android")&&palette.equals("original")?(dark?"#17171e":"#faf9ff"):PresentationPalette.edges(PresentationPalette.valid(palette)?palette:"original",dark,style.equals("glass"))[0]) : dark ? 0xFF18191C : Color.WHITE;
        int text = dark ? 0xFFEDEDF0 : 0xFF222326;
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(background);
        getWindow().getDecorView().setBackgroundColor(background);
        getWindow().setStatusBarColor(Color.TRANSPARENT);
        getWindow().setNavigationBarColor(Color.TRANSPARENT);
        if (Build.VERSION.SDK_INT >= 29) {
            getWindow().setStatusBarContrastEnforced(false);
            getWindow().setNavigationBarContrastEnforced(false);
        }
        WindowCompat.setDecorFitsSystemWindows(getWindow(), false);
        WindowInsetsControllerCompat bars = WindowCompat.getInsetsController(getWindow(), root);
        bars.setAppearanceLightStatusBars(!dark);
        bars.setAppearanceLightNavigationBars(!dark);
        ViewCompat.setOnApplyWindowInsetsListener(root, (view, insets) -> {
            androidx.core.graphics.Insets padding = insets.getInsets(
                    WindowInsetsCompat.Type.systemBars() | WindowInsetsCompat.Type.displayCutout()
                    | WindowInsetsCompat.Type.ime());
            view.setPadding(padding.left, padding.top, padding.right, padding.bottom);
            return insets;
        });
        LinearLayout toolbar = new LinearLayout(this);
        toolbar.setGravity(Gravity.CENTER_VERTICAL);
        toolbar.setPadding(dp(4), dp(4), dp(4), dp(4));
        Button back = button("‹", text, "返回上一网页");
        back.setTextSize(30);
        back.setOnClickListener(v -> goBackOrClose());
        toolbar.addView(back, new LinearLayout.LayoutParams(dp(44), dp(48)));
        LinearLayout title = new LinearLayout(this);
        title.setOrientation(LinearLayout.VERTICAL);
        TextView name = new TextView(this);
        name.setText(browserTitle());
        name.setTextSize(16);
        name.setTypeface(null, Typeface.BOLD);
        name.setTextColor(text);
        title.addView(name);
        host = new TextView(this);
        host.setText(Uri.parse(entryUrl()).getHost());
        host.setTextSize(11);
        host.setTextColor(dark ? 0xFFA1A3AB : 0xFF72757A);
        host.setSingleLine(true);
        host.setEllipsize(TextUtils.TruncateAt.MIDDLE);
        title.addView(host);
        toolbar.addView(title, new LinearLayout.LayoutParams(0, -2, 1));
        Button reload = button("刷新", text, "刷新网页");
        reload.setOnClickListener(v -> reload());
        toolbar.addView(reload, new LinearLayout.LayoutParams(dp(56), dp(48)));
        Button close = button("关闭", text, closeDescription());
        close.setOnClickListener(v -> finish());
        toolbar.addView(close, new LinearLayout.LayoutParams(dp(56), dp(48)));
        root.addView(toolbar);
        progress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal);
        progress.setMax(100);
        root.addView(progress, new LinearLayout.LayoutParams(-1, dp(2)));

        errorPanel = new LinearLayout(this);
        errorPanel.setOrientation(LinearLayout.VERTICAL);
        errorPanel.setGravity(Gravity.CENTER);
        errorPanel.setPadding(dp(24), dp(24), dp(24), dp(24));
        errorText = new TextView(this);
        errorText.setTextColor(text);
        errorText.setGravity(Gravity.CENTER);
        errorPanel.addView(errorText);
        Button retry = button("重试", text, "重试打开网页");
        retry.setOnClickListener(v -> reload());
        errorPanel.addView(retry);
        if (guideExtras()) {
            Button external = button("用外部浏览器打开", text, "用外部浏览器打开官网");
            external.setOnClickListener(v -> { try { if (safeWebUrl(retryUrl)) startActivity(new android.content.Intent(android.content.Intent.ACTION_VIEW, Uri.parse(retryUrl))); } catch (Exception ignored) { Toast.makeText(this,"暂时无法打开浏览器",Toast.LENGTH_SHORT).show(); } });
            errorPanel.addView(external);
        }
        root.addView(errorPanel, new LinearLayout.LayoutParams(-1, 0, 1));
        errorPanel.setVisibility(View.GONE);

        if ((getApplicationInfo().flags & android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE) != 0) {
            WebView.setWebContentsDebuggingEnabled(true);
        }
        web = new WebView(this);
        web.setBackgroundColor(background);
        WebSettings settings = web.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSupportMultipleWindows(false);
        settings.setUseWideViewPort(true);
        settings.setBuiltInZoomControls(true);
        settings.setDisplayZoomControls(false);
        // Unlike the school's temporary login view, this browser retains its
        // own website storage. It never exports or reads cookie values.
        CookieManager.getInstance().setAcceptCookie(true);
        web.setWebChromeClient(new WebChromeClient() {
            @Override public void onShowCustomView(View view,CustomViewCallback callback) {
                if (!guideExtras() || fullScreen != null) { callback.onCustomViewHidden(); return; }
                fullScreen=view;fullScreenCallback=callback;
                ((android.widget.FrameLayout)getWindow().getDecorView()).addView(view,new android.widget.FrameLayout.LayoutParams(-1,-1));
                WindowCompat.getInsetsController(getWindow(),view).hide(WindowInsetsCompat.Type.systemBars());
            }
            @Override public void onHideCustomView() { hideFullScreen(); }
            @Override public void onProgressChanged(WebView view, int value) {
                progress.setProgress(value);
                progress.setVisibility(value < 100 && errorPanel.getVisibility() != View.VISIBLE ? View.VISIBLE : View.GONE);
            }
        });
        web.setWebViewClient(new WebViewClient() {
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return blockUnsupported(request.getUrl().toString());
            }
            @Override public boolean shouldOverrideUrlLoading(WebView view, String url) {
                return blockUnsupported(url);
            }
            @Override public void onPageStarted(WebView view, String url, android.graphics.Bitmap icon) {
                if (safeWebUrl(url)) {
                    retryUrl = url;
                    host.setText(Uri.parse(url).getHost());
                }
                errorPanel.setVisibility(View.GONE);
                web.setVisibility(View.VISIBLE);
                progress.setVisibility(View.VISIBLE);
            }
            @Override public void onPageFinished(WebView view, String url) {
                progress.setVisibility(View.GONE);
            }
            @Override public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (request.isForMainFrame()) showError("网页暂时无法打开，请检查网络后重试。");
            }
            @Override public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                handler.cancel();
                showError("网页安全连接未通过，请稍后重试。");
            }
        });
        root.addView(web, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(root);
        ViewCompat.requestApplyInsets(root);
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override public void handleOnBackPressed() { goBackOrClose(); }
        });
        if (state == null || web.restoreState(state) == null) web.loadUrl(entryUrl());
    }

    static boolean safeWebUrl(String value) {
        try {
            java.net.URI uri = new java.net.URI(value);
            return "https".equalsIgnoreCase(uri.getScheme()) && uri.getHost() != null
                    && uri.getRawUserInfo() == null;
        } catch (Exception ignored) { return false; }
    }
    private boolean blockUnsupported(String url) {
        if (safeWebUrl(url)) return false;
        Toast.makeText(this, "此链接暂不支持在内置网页中打开。", Toast.LENGTH_SHORT).show();
        return true;
    }
    private Button button(String label, int color, String description) {
        Button button = new Button(this);
        button.setText(label);
        button.setTextSize(13);
        button.setTextColor(color);
        button.setContentDescription(description);
        button.setBackgroundColor(Color.TRANSPARENT);
        button.setPadding(0, 0, 0, 0);
        button.setMinWidth(0);
        return button;
    }
    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }
    private void reload() {
        errorPanel.setVisibility(View.GONE);
        web.setVisibility(View.VISIBLE);
        web.loadUrl(retryUrl);
    }
    private void showError(String message) {
        progress.setVisibility(View.GONE);
        web.setVisibility(View.GONE);
        errorText.setText(message);
        errorPanel.setVisibility(View.VISIBLE);
    }
    private void goBackOrClose() {
        if (fullScreen != null) { hideFullScreen(); return; }
        if (web != null && web.canGoBack()) {
            errorPanel.setVisibility(View.GONE);
            web.setVisibility(View.VISIBLE);
            web.goBack();
        } else finish();
    }
    private void hideFullScreen() {
        if(fullScreen==null)return;
        ((android.view.ViewGroup)fullScreen.getParent()).removeView(fullScreen);fullScreen=null;
        if(fullScreenCallback!=null){fullScreenCallback.onCustomViewHidden();fullScreenCallback=null;}
        WindowCompat.getInsetsController(getWindow(),getWindow().getDecorView()).show(WindowInsetsCompat.Type.systemBars());
    }
    @Override protected void onSaveInstanceState(Bundle state) {
        if (web != null) web.saveState(state);
        super.onSaveInstanceState(state);
    }
    @Override protected void onPause() {
        if (web != null) { web.onPause(); CookieManager.getInstance().flush(); }
        super.onPause();
    }
    @Override protected void onResume() {
        super.onResume();
        if (web != null) web.onResume();
    }
    @Override protected void onDestroy() {
        hideFullScreen();
        if (web != null) {
            web.stopLoading();
            if (web.getParent() instanceof android.view.ViewGroup) {
                ((android.view.ViewGroup) web.getParent()).removeView(web);
            }
            web.destroy();
            web = null;
        }
        super.onDestroy();
    }
}
