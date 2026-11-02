package com.xique.assistant;

import androidx.test.platform.app.InstrumentationRegistry;
import androidx.test.filters.SdkSuppress;
import android.content.Context;
import java.io.File;
import java.nio.file.Files;
import java.nio.charset.StandardCharsets;
import org.junit.Test;
import static org.junit.Assert.*;

/** Device-only test. Uses synthetic credentials; never points at the school or API. */
public class SessionVaultDeviceTest {
    @SdkSuppress(minSdkVersion = 26)
    @Test public void encryptionGenerationRecoveryAndReopen() throws Exception {
        Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        SessionVault vault = new SessionVault(context, "device-test");
        long empty = vault.clear();
        long saved = vault.write(empty, "https://api.invalid", "synthetic", "synthetic-token-123");
        assertTrue(saved > empty);
        assertEquals("synthetic-token-123", new SessionVault(context, "device-test").read().getJSONObject("session").getString("access_token"));
        File record = new File(context.getNoBackupFilesDir(), "device-test.enc");
        assertFalse(new String(Files.readAllBytes(record.toPath()), StandardCharsets.ISO_8859_1).contains("synthetic-token-123"));
        try { vault.write(empty, "https://api.invalid", "synthetic", "late"); fail(); }
        catch (SessionVault.Failure failure) { assertEquals("SESSION_CHANGED", failure.code); }
        long cleared = vault.clear();
        assertTrue(cleared > saved);
        assertTrue(vault.read().isNull("session"));
        assertTrue(vault.clear() > cleared);
        Files.write(record.toPath(), new byte[]{1, 2, 3});
        try { vault.read(); fail(); }
        catch (SessionVault.Failure failure) { assertEquals("SESSION_UNREADABLE", failure.code); }
        vault.clear();
        assertTrue(new SessionVault(context, "device-test").read().isNull("session"));
        java.security.KeyStore keys = java.security.KeyStore.getInstance("AndroidKeyStore");
        keys.load(null);
        keys.deleteEntry("xique.device-test");
        try { vault.read(); fail(); }
        catch (SessionVault.Failure failure) { assertEquals("SESSION_UNREADABLE", failure.code); }
        vault.clear();
        assertTrue(vault.read().isNull("session"));
    }
}
