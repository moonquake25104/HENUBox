package com.xique.assistant;

import org.junit.Test;
import static org.junit.Assert.*;
import java.io.*;

public class GuideResourcePolicyTest {
    @Test public void publicGuideOnly(){assertTrue(GuideResources.assetUrl("https://www.henu.edu.cn/images/jmxq.jpg"));assertTrue(GuideResources.assetUrl("https://www.henu.edu.cn/__local/F/D9/test_123.png"));assertTrue(GuideResources.assetUrl(GuideResources.ANTHEM));
        for(String s:new String[]{"http://www.henu.edu.cn/images/a.jpg","https://evil.test/images/a.jpg","https://www.henu.edu.cn.evil.test/images/a.jpg","https://user@www.henu.edu.cn/images/a.jpg","https://www.henu.edu.cn:444/images/a.jpg","https://www.henu.edu.cn/images/../images/a.jpg","https://www.henu.edu.cn/images/%2e%2e/a.jpg","https://www.henu.edu.cn/images/a.jpg?token=x","https://www.henu.edu.cn/images/a.svg","https://www.henu.edu.cn/system/login.jsp","file:///data/private.png",null})assertFalse(String.valueOf(s),GuideResources.assetUrl(s));
        assertEquals(8,GuideResources.PAGES.size());assertFalse(GuideResources.PAGES.containsKey("login"));
    }
    @Test public void responseLimitIsEnforced()throws Exception{ByteArrayOutputStream out=new ByteArrayOutputStream();GuideResources.copyBounded(new ByteArrayInputStream(new byte[1024]),out,1024);assertEquals(1024,out.size());try{GuideResources.copyBounded(new ByteArrayInputStream(new byte[1025]),new ByteArrayOutputStream(),1024);fail("oversized resource accepted");}catch(IOException expected){assertEquals("RESOURCE_TOO_LARGE",expected.getMessage());}}
    @Test public void resourceKeysNeverContainPaths(){String a=GuideResources.hash("https://www.henu.edu.cn/images/a.jpg"),b=GuideResources.hash("https://www.henu.edu.cn/images/b.jpg");assertTrue(a.matches("[a-f0-9]{64}"));assertNotEquals(a,b);assertEquals(a,GuideResources.hash("https://www.henu.edu.cn/images/a.jpg"));}
}
