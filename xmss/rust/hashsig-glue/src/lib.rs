use sha2::{Digest, Sha256};
use ssz::{Decode, Encode};
use std::ffi::CStr;
use std::os::raw::c_char;
use std::ptr;
use std::slice;
use xmss::{
    xmss_key_gen_from_seed, xmss_sign, xmss_verify, XmssPublicKey, XmssSecretKey, XmssSignature,
    MESSAGE_LEN_BYTES,
};

// leanVM-internalized XMSS: a single fixed instantiation (V=42, base 8, log-lifetime 32,
// KoalaBear/Poseidon2). SSZ public key = 32 bytes, signature = 1208 bytes. Keys/sigs from
// leanSig's Dim46 aborting scheme are NOT interoperable with this and must be regenerated.

#[repr(C)]
pub struct PrivateKey {
    inner: XmssSecretKey,
}

#[repr(C)]
pub struct PublicKey {
    pub inner: XmssPublicKey,
}

#[repr(C)]
pub struct Signature {
    pub inner: XmssSignature,
}

#[repr(C)]
pub struct KeyPair {
    pub public_key: PublicKey,
    pub private_key: PrivateKey,
}

/// Deterministic key generation: the seed phrase is SHA-256'd into the 32-byte seed that is
/// the key's entire secret material, so the same phrase + activation range always regenerates
/// the same key. Returns null on an invalid activation range.
#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_generate(
    seed_phrase: *const c_char,
    activation_epoch: usize,
    num_active_epochs: usize,
) -> *mut KeyPair {
    if seed_phrase.is_null() {
        return ptr::null_mut();
    }
    let seed_phrase = unsafe { CStr::from_ptr(seed_phrase).to_string_lossy().into_owned() };
    let mut hasher = Sha256::new();
    hasher.update(seed_phrase.as_bytes());
    let seed: [u8; 32] = hasher.finalize().into();

    match xmss_key_gen_from_seed(seed, activation_epoch as u64, num_active_epochs as u64) {
        Ok((public_key, private_key)) => Box::into_raw(Box::new(KeyPair {
            public_key: PublicKey { inner: public_key },
            private_key: PrivateKey { inner: private_key },
        })),
        Err(_) => ptr::null_mut(),
    }
}

/// Reconstruct a key pair from its persisted parts: the secret key is postcard (serde), the
/// public key is SSZ. The two encodings differ because upstream persists the secret key with
/// serde and deliberately excludes it from SSZ.
#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_from_ssz(
    private_key_ptr: *const u8,
    private_key_len: usize,
    public_key_ptr: *const u8,
    public_key_len: usize,
) -> *mut KeyPair {
    if private_key_ptr.is_null() || public_key_ptr.is_null() {
        return ptr::null_mut();
    }
    unsafe {
        let sk_slice = slice::from_raw_parts(private_key_ptr, private_key_len);
        let pk_slice = slice::from_raw_parts(public_key_ptr, public_key_len);

        let private_key: XmssSecretKey = match postcard::from_bytes(sk_slice) {
            Ok(key) => key,
            Err(_) => return ptr::null_mut(),
        };
        let public_key: XmssPublicKey = match XmssPublicKey::from_ssz_bytes(pk_slice) {
            Ok(key) => key,
            Err(_) => return ptr::null_mut(),
        };

        Box::into_raw(Box::new(KeyPair {
            public_key: PublicKey { inner: public_key },
            private_key: PrivateKey { inner: private_key },
        }))
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_free(keypair: *mut KeyPair) {
    if !keypair.is_null() {
        unsafe {
            let _ = Box::from_raw(keypair);
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_get_public_key(
    keypair: *const KeyPair,
) -> *const PublicKey {
    if keypair.is_null() {
        return ptr::null();
    }
    &(*keypair).public_key
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_get_private_key(
    keypair: *const KeyPair,
) -> *const PrivateKey {
    if keypair.is_null() {
        return ptr::null();
    }
    &(*keypair).private_key
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_public_key_from_ssz(
    public_key_ptr: *const u8,
    public_key_len: usize,
) -> *mut PublicKey {
    if public_key_ptr.is_null() {
        return ptr::null_mut();
    }
    unsafe {
        let pk_slice = slice::from_raw_parts(public_key_ptr, public_key_len);
        let public_key: XmssPublicKey = match XmssPublicKey::from_ssz_bytes(pk_slice) {
            Ok(key) => key,
            Err(_) => return ptr::null_mut(),
        };
        Box::into_raw(Box::new(PublicKey { inner: public_key }))
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_public_key_free(public_key: *mut PublicKey) {
    if !public_key.is_null() {
        unsafe {
            let _ = Box::from_raw(public_key);
        }
    }
}

/// Sign a 32-byte message for the given slot. Signing is derandomized (randomness is derived
/// from the secret key, slot, and message), so repeating a (slot, message) pair is harmless;
/// signing two *different* messages at the same slot must never happen (stateful scheme).
#[no_mangle]
pub unsafe extern "C" fn hashsig_sign(
    private_key: *const PrivateKey,
    message_ptr: *const u8,
    epoch: u32,
) -> *mut Signature {
    if private_key.is_null() || message_ptr.is_null() {
        return ptr::null_mut();
    }
    unsafe {
        let private_key_ref = &*private_key;
        let message_slice = slice::from_raw_parts(message_ptr, MESSAGE_LEN_BYTES);
        let message_array: &[u8; MESSAGE_LEN_BYTES] = match message_slice.try_into() {
            Ok(arr) => arr,
            Err(_) => return ptr::null_mut(),
        };
        match xmss_sign(&private_key_ref.inner, epoch, message_array) {
            Ok(sig) => Box::into_raw(Box::new(Signature { inner: sig })),
            Err(_) => ptr::null_mut(),
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_signature_free(signature: *mut Signature) {
    if !signature.is_null() {
        unsafe {
            let _ = Box::from_raw(signature);
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_signature_from_ssz(
    signature_ptr: *const u8,
    signature_len: usize,
) -> *mut Signature {
    if signature_ptr.is_null() || signature_len == 0 {
        return ptr::null_mut();
    }
    unsafe {
        let sig_slice = slice::from_raw_parts(signature_ptr, signature_len);
        let signature: XmssSignature = match XmssSignature::from_ssz_bytes(sig_slice) {
            Ok(sig) => sig,
            Err(_) => return ptr::null_mut(),
        };
        Box::into_raw(Box::new(Signature { inner: signature }))
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_verify(
    public_key: *const PublicKey,
    message_ptr: *const u8,
    epoch: u32,
    signature: *const Signature,
) -> i32 {
    if public_key.is_null() || message_ptr.is_null() || signature.is_null() {
        return -1;
    }
    unsafe {
        let public_key_ref = &*public_key;
        let signature_ref = &*signature;
        let message_slice = slice::from_raw_parts(message_ptr, MESSAGE_LEN_BYTES);
        let message_array: &[u8; MESSAGE_LEN_BYTES] = match message_slice.try_into() {
            Ok(arr) => arr,
            Err(_) => return -1,
        };
        match xmss_verify(
            &public_key_ref.inner,
            epoch,
            message_array,
            &signature_ref.inner,
        ) {
            Ok(()) => 1,
            Err(_) => 0,
        }
    }
}

#[no_mangle]
pub extern "C" fn hashsig_message_length() -> usize {
    MESSAGE_LEN_BYTES
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_signature_to_bytes(
    signature: *const Signature,
    buffer: *mut u8,
    buffer_len: usize,
) -> usize {
    if signature.is_null() || buffer.is_null() {
        return 0;
    }
    unsafe {
        let sig_ref = &*signature;
        let ssz_bytes = sig_ref.inner.as_ssz_bytes();
        if ssz_bytes.len() > buffer_len {
            return 0;
        }
        let output_slice = slice::from_raw_parts_mut(buffer, buffer_len);
        output_slice[..ssz_bytes.len()].copy_from_slice(&ssz_bytes);
        ssz_bytes.len()
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_public_key_to_bytes(
    public_key: *const PublicKey,
    buffer: *mut u8,
    buffer_len: usize,
) -> usize {
    if public_key.is_null() || buffer.is_null() {
        return 0;
    }
    unsafe {
        let public_key_ref = &*public_key;
        let ssz_bytes = public_key_ref.inner.as_ssz_bytes();
        if ssz_bytes.len() > buffer_len {
            return 0;
        }
        let output_slice = slice::from_raw_parts_mut(buffer, buffer_len);
        output_slice[..ssz_bytes.len()].copy_from_slice(&ssz_bytes);
        ssz_bytes.len()
    }
}

/// Secret key bytes are postcard (serde), not SSZ. Returns 0 on a null pointer, a buffer too
/// small, or a serialization failure.
#[no_mangle]
pub unsafe extern "C" fn hashsig_private_key_to_bytes(
    private_key: *const PrivateKey,
    buffer: *mut u8,
    buffer_len: usize,
) -> usize {
    if private_key.is_null() || buffer.is_null() {
        return 0;
    }
    unsafe {
        let private_key_ref = &*private_key;
        let bytes: Vec<u8> = match postcard::to_allocvec(&private_key_ref.inner) {
            Ok(bytes) => bytes,
            Err(_) => return 0,
        };
        if bytes.len() > buffer_len {
            return 0;
        }
        let output_slice = slice::from_raw_parts_mut(buffer, buffer_len);
        output_slice[..bytes.len()].copy_from_slice(&bytes);
        bytes.len()
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_verify_ssz(
    pubkey_bytes: *const u8,
    pubkey_len: usize,
    message: *const u8,
    epoch: u32,
    signature_bytes: *const u8,
    signature_len: usize,
) -> i32 {
    if pubkey_bytes.is_null() || message.is_null() || signature_bytes.is_null() {
        return -1;
    }
    unsafe {
        let pk_data = slice::from_raw_parts(pubkey_bytes, pubkey_len);
        let sig_data = slice::from_raw_parts(signature_bytes, signature_len);
        let msg_data = slice::from_raw_parts(message, MESSAGE_LEN_BYTES);
        let message_array: &[u8; MESSAGE_LEN_BYTES] = match msg_data.try_into() {
            Ok(arr) => arr,
            Err(_) => return -1,
        };
        let pk: XmssPublicKey = match XmssPublicKey::from_ssz_bytes(pk_data) {
            Ok(pk) => pk,
            Err(_) => return -1,
        };
        let sig: XmssSignature = match XmssSignature::from_ssz_bytes(sig_data) {
            Ok(sig) => sig,
            Err(_) => return -1,
        };
        match xmss_verify(&pk, epoch, message_array, &sig) {
            Ok(()) => 1,
            Err(_) => 0,
        }
    }
}
