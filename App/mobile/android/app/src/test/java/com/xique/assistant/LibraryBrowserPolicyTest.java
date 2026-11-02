package com.xique.assistant;

import org.junit.Test;
import static org.junit.Assert.*;

public class LibraryBrowserPolicyTest {
    @Test public void officialEntryAndHttpsLoginRedirectsAreSupported() {
        assertEquals("https://zwyy.henu.edu.cn/h5/index.html#/mo/home", LibraryBrowserActivity.HOME);
        assertTrue(LibraryBrowserActivity.safeWebUrl(LibraryBrowserActivity.HOME));
        assertTrue(LibraryBrowserActivity.safeWebUrl("https://ids.henu.edu.cn/login?service=library"));
    }
    @Test public void websiteLinksCannotOpenFilesOrAppIntents() {
        for (String url : new String[]{null, "", "http://zwyy.henu.edu.cn/", "file:///data/data/a",
                "content://private", "javascript:alert(1)", "intent://launch", "https://user:pass@example.com",
                "https:example.com", "https://"}) {
            assertFalse(LibraryBrowserActivity.safeWebUrl(url));
        }
    }
    @Test public void emailEntryPreservesTheOfficialNestedService() throws Exception {
        assertTrue(LibraryBrowserActivity.safeWebUrl(EmailBrowserActivity.EMAIL_HOME));
        java.net.URI url = new java.net.URI(EmailBrowserActivity.EMAIL_HOME);
        assertEquals("ids.henu.edu.cn",url.getHost());
        assertEquals("/authserver/login",url.getPath());
        String service = java.net.URLDecoder.decode(url.getRawQuery().substring("service=".length()),"UTF-8");
        assertEquals("https://ehall.henu.edu.cn:443/login?service=https://ehall.henu.edu.cn/appShow?appId=5918811960262548",service);
    }
}
