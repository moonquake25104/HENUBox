package com.xique.assistant;

import android.content.Context;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.AtomicFile;
import java.io.File;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.security.MessageDigest;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import org.json.JSONObject;

/** One process, one serialized writer. Neither credential plaintext nor keys reach disk. */
final class SessionVault {
    static final Object LOCK = new Object();
    private final Context context;
    private final boolean primary;
    private final String alias;
    private static final byte[] AAD = "com.xique.assistant/session/v1".getBytes(StandardCharsets.UTF_8);
    private final AtomicFile record;
    // Non-secret high-water counter permits explicit clear after ciphertext/key loss.
    // Reserve before committing the encrypted record; gaps after crashes are intentional.
    private final AtomicFile counter;

    static final class Failure extends Exception {
        final String code;
        Failure(String code) { super(code); this.code = code; }
    }

    SessionVault(Context context) {
        this(context, "session-v1");
    }

    SessionVault(Context context, String namespace) {
        this.context = context;
        primary = namespace.equals("session-v1");
        if (!namespace.matches("[a-z0-9-]+")) throw new IllegalArgumentException();
        alias = "xique." + namespace;
        File dir = context.getNoBackupFilesDir();
        record = new AtomicFile(new File(dir, namespace + ".enc"));
        counter = new AtomicFile(new File(dir, namespace + "-generation"));
    }

    JSONObject read() throws Failure {
        synchronized (LOCK) { return readLocked(); }
    }

    long write(long expected, String origin, String account, String token) throws Failure {
        synchronized (LOCK) {
            JSONObject current = readLocked();
            if (current.optLong("generation", -1) != expected) throw new Failure("SESSION_CHANGED");
            try {
                JSONObject session = new JSONObject().put("server_origin", SessionPolicy.origin(origin))
                    .put("account_id", SessionPolicy.text(account, 1, 256))
                    .put("access_token", SessionPolicy.text(token, 1, 32768));
                JSONObject previous = current.optJSONObject("session");
                if (primary && (previous == null || !origin.equals(previous.optString("server_origin"))
                    || !account.equals(previous.optString("account_id")))) new TimetableStore(context).clear();
                return commitLocked(session);
            } catch (IllegalArgumentException e) { throw new Failure("INVALID_ARGUMENT"); }
            catch (Failure e) { throw e; }
            catch (Exception e) { throw new Failure("STORAGE_FAILURE"); }
        }
    }

    long clear() throws Failure {
        synchronized (LOCK) {
            // Deliberately independent of decryption: user can recover from key loss.
            if (primary) new TimetableStore(context).clear();
            return commitLocked(JSONObject.NULL);
        }
    }

    private long counterLocked() throws Exception {
        if (!exists(counter)) {
            if (exists(record)) throw new Exception("Missing counter");
            return 0;
        }
        return SessionPolicy.generation(Long.parseLong(new String(counter.readFully(), StandardCharsets.US_ASCII)));
    }

    private JSONObject readLocked() throws Failure {
        try {
            long reserved = counterLocked();
            if (!exists(record)) {
                if (reserved != 0) throw new Exception("Incomplete write");
                return new JSONObject().put("generation", 0).put("session", JSONObject.NULL);
            }
            byte[] envelope = record.readFully();
            if (envelope.length < 29 || envelope.length > 200000 || envelope[0] != 1) throw new Exception("Envelope");
            KeyStore keys = KeyStore.getInstance("AndroidKeyStore");
            keys.load(null);
            SecretKey key = (SecretKey) keys.getKey(alias, null);
            if (key == null) throw new Exception("Key lost");
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            cipher.init(Cipher.DECRYPT_MODE, key, new GCMParameterSpec(128, envelope, 1, 12));
            cipher.updateAAD(AAD);
            JSONObject result = new JSONObject(new String(cipher.doFinal(envelope, 13, envelope.length - 13), StandardCharsets.UTF_8));
            long generation = SessionPolicy.generation(result.get("generation"));
            if (generation == 0 || generation > reserved) throw new Exception("Generation");
            if (!result.isNull("session")) {
                JSONObject session = result.getJSONObject("session");
                SessionPolicy.origin(session.get("server_origin"));
                SessionPolicy.text(session.get("account_id"), 1, 256);
                SessionPolicy.text(session.get("access_token"), 1, 32768);
            }
            return result;
        } catch (Exception e) { throw new Failure("SESSION_UNREADABLE"); }
    }

    private long commitLocked(Object session) throws Failure {
        try {
            long next = counterLocked() + 1;
            SessionPolicy.generation(next);
            KeyStore keys = KeyStore.getInstance("AndroidKeyStore");
            keys.load(null);
            SecretKey key = (SecretKey) keys.getKey(alias, null);
            if (key == null) {
                KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
                generator.init(new KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setKeySize(256).setRandomizedEncryptionRequired(true).build());
                key = generator.generateKey();
            }
            JSONObject value = new JSONObject().put("generation", next).put("session", session);
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            cipher.init(Cipher.ENCRYPT_MODE, key);
            cipher.updateAAD(AAD);
            byte[] iv = cipher.getIV();
            if (iv.length != 12) throw new Exception("IV");
            byte[] encrypted = cipher.doFinal(value.toString().getBytes(StandardCharsets.UTF_8));
            byte[] envelope = new byte[13 + encrypted.length];
            envelope[0] = 1;
            System.arraycopy(iv, 0, envelope, 1, 12);
            System.arraycopy(encrypted, 0, envelope, 13, encrypted.length);
            atomicWrite(counter, Long.toString(next).getBytes(StandardCharsets.US_ASCII));
            atomicWrite(record, envelope);
            return next;
        } catch (Exception e) { throw new Failure("STORAGE_FAILURE"); }
    }

    private static void atomicWrite(AtomicFile target, byte[] bytes) throws Exception {
        FileOutputStream output = null;
        try {
            output = target.startWrite();
            output.write(bytes);
            output.getFD().sync();
            target.finishWrite(output);
            output = null;
            if (!MessageDigest.isEqual(bytes, target.readFully())) throw new Exception("Commit failed");
        } catch (Exception e) {
            if (output != null) target.failWrite(output);
            throw e;
        }
    }

    private static boolean exists(AtomicFile file) {
        return file.getBaseFile().exists() || new File(file.getBaseFile().getPath() + ".bak").exists();
    }
}
