package com.xique.assistant;

import com.getcapacitor.BridgeActivity;
import android.os.Bundle;
import android.os.Build;
import android.graphics.Color;
import android.graphics.drawable.ColorDrawable;
import android.graphics.drawable.GradientDrawable;
import android.content.res.Configuration;
import android.view.View;
import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsControllerCompat;

public class MainActivity extends BridgeActivity {
    com.henubox.glass.BackdropNavHost backdropHost;
    com.henubox.glass.BackdropNavHost ensureBackdropHost(){if(backdropHost==null)backdropHost=com.henubox.glass.BackdropNavHost.install(this,bridge.getWebView());return backdropHost;}
    @Override public void onDestroy(){if(backdropHost!=null){backdropHost.dispose();backdropHost=null;}super.onDestroy();}
    @Override public void onNewIntent(android.content.Intent intent) { super.onNewIntent(intent);setIntent(intent);if(CourseReminders.OPEN.equals(intent.getAction())&&bridge!=null&&SessionPolicy.trustedPage(bridge.getWebView().getUrl()))bridge.getWebView().post(()->bridge.getWebView().evaluateJavascript("window.location.hash='#/timetable'",null)); }
    @Override
    public void onCreate(Bundle state) {
        registerPlugin(XiqueSessionPlugin.class);
        registerPlugin(XiqueTimetablePlugin.class);
        registerPlugin(XiqueLocalPlugin.class);
        registerPlugin(XiqueHenuPlugin.class);
        registerPlugin(XiqueClassroomPlugin.class);
        registerPlugin(XiqueLibraryPlugin.class);
        registerPlugin(XiqueEmailPlugin.class);
        registerPlugin(XiqueFocusPlugin.class);
        registerPlugin(XiqueFocusExtrasPlugin.class);
        registerPlugin(XiqueTodoPlugin.class);
        registerPlugin(XiqueAppearancePlugin.class);
        registerPlugin(XiqueBackdropPlugin.class);
        registerPlugin(XiqueCourseRemindersPlugin.class);
        registerPlugin(XiqueGuidePlugin.class);
        super.onCreate(state);
        restoreAppAppearance();
    }

    void setAppAppearance(String theme) {
        setAppAppearance(theme,getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("style","classic"));
    }
    void setAppAppearance(String theme,String style) {
        setAppAppearance(theme,style,getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("palette","original"));
    }
    void setAppAppearance(String theme,String style,String palette) {
        setAppAppearance(theme,style,palette,theme);
    }
    void setAppAppearance(String theme,String style,String palette,String mode) {
        if(!PresentationPalette.valid(palette))throw new IllegalArgumentException("INVALID_ARGUMENT");
        if(!mode.equals("light")&&!mode.equals("dark")&&!mode.equals("system"))throw new IllegalArgumentException("INVALID_ARGUMENT");
        getSharedPreferences("xique-appearance",MODE_PRIVATE).edit().putString("theme",theme).putString("style",style).putString("palette",palette).putString("mode",mode).apply();
        applyAppAppearance(theme.equals("dark"),style,palette);
    }
    private void restoreAppAppearance() {
        String fallback=(getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK)==Configuration.UI_MODE_NIGHT_YES?"dark":"light";
        String palette=getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("palette","original");
        String theme="system".equals(getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("mode",""))?fallback:getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("theme",fallback);
        applyAppAppearance(theme.equals("dark"),getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("style","classic"),PresentationPalette.valid(palette)?palette:"original");
    }
    private android.graphics.drawable.Drawable hostBackground(int top,int bottom,boolean glass){return glass?new GradientDrawable(GradientDrawable.Orientation.TOP_BOTTOM,new int[]{top,top,bottom,bottom}):new ColorDrawable(top);}
    @SuppressWarnings("deprecation") private void applyAppAppearance(boolean dark,String style,String palette) {
        boolean glass=style.equals("glass");
        // Match the solid feathered edges of liquid-glass.css, including Android15 edge-to-edge margins.
        String[] edges=style.equals("android")&&palette.equals("original")?new String[]{dark?"#17171e":"#faf9ff",dark?"#17171e":"#faf9ff"}:PresentationPalette.edges(palette,dark,glass);
        int color=Color.parseColor(edges[0]),bottom=Color.parseColor(edges[1]);
        getWindow().setBackgroundDrawable(hostBackground(color,bottom,glass));
        getWindow().getDecorView().setBackground(hostBackground(color,bottom,glass));
        View content=findViewById(android.R.id.content);
        if(content!=null)content.setBackground(hostBackground(color,bottom,glass));
        // Android 15+ draws transparent bars over this host behind the WebView
        // margins. Coloring only the CSS or statusBarColor cannot color that host.
        getWindow().setStatusBarColor(color);
        getWindow().setNavigationBarColor(bottom);
        if(Build.VERSION.SDK_INT>=29) {
            getWindow().setStatusBarContrastEnforced(false);
            getWindow().setNavigationBarContrastEnforced(false);
        }
        WindowInsetsControllerCompat bars=WindowCompat.getInsetsController(getWindow(),getWindow().getDecorView());
        bars.setAppearanceLightStatusBars(!dark);
        bars.setAppearanceLightNavigationBars(!dark);
        if(bridge!=null && bridge.getWebView()!=null)bridge.getWebView().setBackgroundColor(color);
    }
    @Override public void onResume() { super.onResume();restoreAppAppearance();CourseReminders.refreshAsync(this); }
    @Override public void onConfigurationChanged(Configuration config) { super.onConfigurationChanged(config);restoreAppAppearance(); }
}
