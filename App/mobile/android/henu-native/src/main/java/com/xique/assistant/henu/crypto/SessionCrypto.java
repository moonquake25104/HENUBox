package com.xique.assistant.henu.crypto;

import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;

import java.security.KeyStore;
import java.security.SecureRandom;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/**
 * AES-256-GCM session sealing backed by the Android Keystore. The envelope is
 * [version(1)|iv(12)|ciphertext+tag]; the AAD binds workspace/school/
 * generation/schema so a blob copied to another row or workspace fails to
 * open. Key loss or tampering surfaces as SessionUnreadable — callers must
 * treat that as "re-login required", never as an empty session.
 */
public final class SessionCrypto {

    public static final int SCHEMA = 1;
    private static final String KEYSTORE = "AndroidKeyStore";
    private static final String ALIAS = "xique.henu.session.v1";
    private static final int IV_LEN = 12;
    private static final int TAG_BITS = 128;

    private static final Object LOCK = new Object();
    private static SecretKey cachedKey;
    private static boolean keyFailed;

    public static final class SessionUnreadable extends Exception {
        public SessionUnreadable(String message) {
            super(message);
        }
    }

    private SessionCrypto() {
    }

    public static byte[] seal(byte[] plaintext, String aad) throws SessionUnreadable {
        try {
            SecretKey key = key();
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            cipher.init(Cipher.ENCRYPT_MODE, key);
            byte[] iv = cipher.getIV();
            if (iv == null || iv.length != IV_LEN) {
                iv = new byte[IV_LEN];
                new SecureRandom().nextBytes(iv);
                cipher.init(Cipher.ENCRYPT_MODE, key, new GCMParameterSpec(TAG_BITS, iv));
            }
            cipher.updateAAD(aadBytes(aad));
            byte[] ct = cipher.doFinal(plaintext);
            byte[] out = new byte[1 + IV_LEN + ct.length];
            out[0] = SCHEMA;
            System.arraycopy(iv, 0, out, 1, IV_LEN);
            System.arraycopy(ct, 0, out, 1 + IV_LEN, ct.length);
            return out;
        } catch (SessionUnreadable e) {
            throw e;
        } catch (Exception e) {
            throw new SessionUnreadable("seal failed");
        }
    }

    public static byte[] open(byte[] envelope, String aad) throws SessionUnreadable {
        if (envelope == null || envelope.length < 1 + IV_LEN + TAG_BITS / 8
                || envelope[0] != SCHEMA) {
            throw new SessionUnreadable("bad envelope");
        }
        try {
            SecretKey key = key();
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            cipher.init(Cipher.DECRYPT_MODE, key, new GCMParameterSpec(TAG_BITS,
                    envelope, 1, IV_LEN));
            cipher.updateAAD(aadBytes(aad));
            return cipher.doFinal(envelope, 1 + IV_LEN, envelope.length - 1 - IV_LEN);
        } catch (SessionUnreadable e) {
            throw e;
        } catch (Exception e) {
            // Wrong key (Keystore wiped, device migrated) and tampering both
            // land here on purpose.
            throw new SessionUnreadable("open failed");
        }
    }

    public static String aad(String workspace, String school, long generation) {
        return "henu-session-v" + SCHEMA + "|" + workspace + "|" + school + "|" + generation;
    }

    private static byte[] aadBytes(String aad) {
        return aad.getBytes(java.nio.charset.StandardCharsets.UTF_8);
    }

    private static SecretKey key() throws SessionUnreadable {
        synchronized (LOCK) {
            if (cachedKey != null) return cachedKey;
            if (keyFailed) throw new SessionUnreadable("keystore unavailable");
            try {
                KeyStore ks = KeyStore.getInstance(KEYSTORE);
                ks.load(null);
                KeyStore.Entry entry = ks.getEntry(ALIAS, null);
                if (entry instanceof KeyStore.SecretKeyEntry) {
                    cachedKey = ((KeyStore.SecretKeyEntry) entry).getSecretKey();
                    return cachedKey;
                }
                KeyGenerator gen = KeyGenerator.getInstance(
                        KeyProperties.KEY_ALGORITHM_AES, KEYSTORE);
                gen.init(new KeyGenParameterSpec.Builder(ALIAS,
                        KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                        .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                        .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                        .setKeySize(256)
                        .setRandomizedEncryptionRequired(true)
                        .build());
                cachedKey = gen.generateKey();
                return cachedKey;
            } catch (Exception e) {
                keyFailed = true;
                throw new SessionUnreadable("keystore init failed");
            }
        }
    }
}
