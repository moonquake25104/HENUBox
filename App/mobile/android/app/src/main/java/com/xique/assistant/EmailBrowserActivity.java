package com.xique.assistant;

/** Fixed official email self-service page; user performs login and opening. */
public final class EmailBrowserActivity extends LibraryBrowserActivity {
    static final String EMAIL_HOME = "https://ids.henu.edu.cn/authserver/login?service=https%3A%2F%2Fehall.henu.edu.cn%3A443%2Flogin%3Fservice%3Dhttps%3A%2F%2Fehall.henu.edu.cn%2FappShow%3FappId%3D5918811960262548";
    @Override protected String entryUrl() { return EMAIL_HOME; }
    @Override protected String browserTitle() { return "开通学校邮箱"; }
    @Override protected String closeDescription() { return "关闭邮箱开户网页"; }
}
