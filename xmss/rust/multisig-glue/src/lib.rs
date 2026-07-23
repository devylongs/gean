use backend::symmetric::Permutation;
use backend::{
    default_koalabear_poseidon1_16, default_koalabear_poseidon1_24, KoalaBear, PrimeField32,
};
use lean_multisig::{
    aggregate_single_message_signatures, merge_single_message_aggregates, setup_prover,
    setup_verifier, verify_multi_message_aggregate, verify_single_message_aggregate,
    MultiMessageAggregateSignature, SingleMessageAggregateSignature, XmssPublicKey, XmssSignature,
};
use rec_aggregation::split_multi_message_aggregate_by_message;
use std::panic::AssertUnwindSafe;
use std::slice;
use std::sync::OnceLock;

#[repr(C)]
pub struct PublicKey {
    pub inner: XmssPublicKey,
}

#[repr(C)]
pub struct Signature {
    pub inner: XmssSignature,
}

const MESSAGE_LEN: usize = 32;

static PROVER_READY: OnceLock<bool> = OnceLock::new();
static VERIFIER_READY: OnceLock<bool> = OnceLock::new();

macro_rules! ffi_guard {
    ($fallback:expr, $body:block) => {
        std::panic::catch_unwind(AssertUnwindSafe(|| $body)).unwrap_or($fallback)
    };
}

// setup_prover enables a process-wide arena allocator and warms the prover.
// Its single shared region means two proofs must never be generated
// concurrently; the Go-side proving.Gate serializes all aggregate/merge/split
// work to one at a time, so that invariant holds. Verification does not use the
// arena and stays safe to run concurrently.
#[no_mangle]
pub extern "C" fn xmss_setup_prover() -> i32 {
    let ready = PROVER_READY.get_or_init(|| std::panic::catch_unwind(setup_prover).is_ok());
    if *ready {
        0
    } else {
        -1
    }
}

#[no_mangle]
pub extern "C" fn xmss_setup_verifier() -> i32 {
    let ready = VERIFIER_READY.get_or_init(|| std::panic::catch_unwind(setup_verifier).is_ok());
    if *ready {
        0
    } else {
        -1
    }
}

unsafe fn write_out(src: &[u8], out: *mut u8, cap: usize, written: *mut usize) -> i32 {
    if written.is_null() {
        return -1;
    }
    *written = src.len();
    if src.len() > cap {
        return -2;
    }
    if !src.is_empty() {
        if out.is_null() {
            return -1;
        }
        std::ptr::copy_nonoverlapping(src.as_ptr(), out, src.len());
    }
    0
}

unsafe fn collect_pubkeys(
    ptrs: *const *const PublicKey,
    count: usize,
) -> Option<Vec<XmssPublicKey>> {
    if count > 0 && ptrs.is_null() {
        return None;
    }
    let mut keys = Vec::with_capacity(count);
    for &ptr in slice::from_raw_parts(ptrs, count) {
        if ptr.is_null() {
            return None;
        }
        keys.push((*ptr).inner.clone());
    }
    Some(keys)
}

unsafe fn collect_key_groups(
    flat: *const *const PublicKey,
    counts: *const usize,
    group_count: usize,
) -> Option<Vec<Vec<XmssPublicKey>>> {
    if group_count == 0 || flat.is_null() || counts.is_null() {
        return None;
    }
    let counts = slice::from_raw_parts(counts, group_count);
    let mut groups = Vec::with_capacity(group_count);
    let mut offset = 0usize;
    for &count in counts {
        groups.push(collect_pubkeys(flat.add(offset), count)?);
        offset = offset.checked_add(count)?;
    }
    Some(groups)
}

#[no_mangle]
pub unsafe extern "C" fn xmss_aggregate_type_1(
    raw_pub_keys: *const *const PublicKey,
    raw_signatures: *const *const Signature,
    num_raw: usize,
    child_all_pub_keys: *const *const PublicKey,
    child_num_keys: *const usize,
    child_proof_ptrs: *const *const u8,
    child_proof_lens: *const usize,
    num_children: usize,
    message_hash: *const u8,
    slot: u32,
    log_inv_rate: usize,
    out: *mut u8,
    cap: usize,
    written: *mut usize,
) -> i32 {
    ffi_guard!(-1, {
        if message_hash.is_null()
            || written.is_null()
            || (num_raw > 0 && (raw_pub_keys.is_null() || raw_signatures.is_null()))
            || (num_children > 0
                && (child_all_pub_keys.is_null()
                    || child_num_keys.is_null()
                    || child_proof_ptrs.is_null()
                    || child_proof_lens.is_null()))
        {
            return -1;
        }
        let message: [u8; MESSAGE_LEN] =
            match slice::from_raw_parts(message_hash, MESSAGE_LEN).try_into() {
                Ok(message) => message,
                Err(_) => return -1,
            };

        let mut raw = Vec::with_capacity(num_raw);
        if num_raw > 0 {
            let keys = slice::from_raw_parts(raw_pub_keys, num_raw);
            let signatures = slice::from_raw_parts(raw_signatures, num_raw);
            for i in 0..num_raw {
                if keys[i].is_null() || signatures[i].is_null() {
                    return -1;
                }
                raw.push(((*keys[i]).inner.clone(), (*signatures[i]).inner.clone()));
            }
        }

        let mut children = Vec::with_capacity(num_children);
        if num_children > 0 {
            let counts = slice::from_raw_parts(child_num_keys, num_children);
            let proofs = slice::from_raw_parts(child_proof_ptrs, num_children);
            let lengths = slice::from_raw_parts(child_proof_lens, num_children);
            let mut offset = 0usize;
            for i in 0..num_children {
                let keys = match collect_pubkeys(child_all_pub_keys.add(offset), counts[i]) {
                    Some(keys) => keys,
                    None => return -1,
                };
                offset = match offset.checked_add(counts[i]) {
                    Some(offset) => offset,
                    None => return -1,
                };
                if proofs[i].is_null() || lengths[i] == 0 {
                    return -1;
                }
                let proof = slice::from_raw_parts(proofs[i], lengths[i]);
                match SingleMessageAggregateSignature::decompress_without_pubkeys(proof, keys) {
                    Some(proof) => children.push(proof),
                    None => return -1,
                }
            }
        }

        let proof = match std::panic::catch_unwind(AssertUnwindSafe(|| {
            aggregate_single_message_signatures(&children, raw, message, slot, log_inv_rate)
        })) {
            Ok(Ok(proof)) => proof,
            _ => return -1,
        };
        write_out(&proof.compress_without_pubkeys(), out, cap, written)
    })
}

#[no_mangle]
pub unsafe extern "C" fn xmss_verify_type_1(
    public_keys: *const *const PublicKey,
    num_keys: usize,
    message_hash: *const u8,
    slot: u32,
    proof: *const u8,
    proof_len: usize,
) -> bool {
    ffi_guard!(false, {
        if message_hash.is_null() || proof.is_null() || proof_len == 0 {
            return false;
        }
        let message: [u8; MESSAGE_LEN] =
            match slice::from_raw_parts(message_hash, MESSAGE_LEN).try_into() {
                Ok(message) => message,
                Err(_) => return false,
            };
        let keys = match collect_pubkeys(public_keys, num_keys) {
            Some(keys) => keys,
            None => return false,
        };
        let proof = match SingleMessageAggregateSignature::decompress_without_pubkeys(
            slice::from_raw_parts(proof, proof_len),
            keys,
        ) {
            Some(proof) => proof,
            None => return false,
        };
        if proof.info.without_pubkeys.message != message || proof.info.without_pubkeys.slot != slot
        {
            return false;
        }
        verify_single_message_aggregate(&proof).is_ok()
    })
}

#[no_mangle]
pub unsafe extern "C" fn xmss_merge_type_1_to_type_2(
    proof_ptrs: *const *const u8,
    proof_lens: *const usize,
    pubkeys: *const *const PublicKey,
    pubkey_counts: *const usize,
    count: usize,
    log_inv_rate: usize,
    out: *mut u8,
    cap: usize,
    written: *mut usize,
) -> i32 {
    ffi_guard!(-1, {
        if count == 0
            || proof_ptrs.is_null()
            || proof_lens.is_null()
            || pubkeys.is_null()
            || pubkey_counts.is_null()
            || written.is_null()
        {
            return -1;
        }
        let proof_ptrs = slice::from_raw_parts(proof_ptrs, count);
        let proof_lens = slice::from_raw_parts(proof_lens, count);
        let groups = match collect_key_groups(pubkeys, pubkey_counts, count) {
            Some(groups) => groups,
            None => return -1,
        };
        let mut proofs = Vec::with_capacity(count);
        for i in 0..count {
            if proof_ptrs[i].is_null() || proof_lens[i] == 0 {
                return -1;
            }
            match SingleMessageAggregateSignature::decompress_without_pubkeys(
                slice::from_raw_parts(proof_ptrs[i], proof_lens[i]),
                groups[i].clone(),
            ) {
                Some(proof) => proofs.push(proof),
                None => return -1,
            }
        }
        let proof = match std::panic::catch_unwind(AssertUnwindSafe(|| {
            merge_single_message_aggregates(proofs, log_inv_rate)
        })) {
            Ok(Ok(proof)) => proof,
            _ => return -1,
        };
        write_out(&proof.compress_without_pubkeys(), out, cap, written)
    })
}

#[no_mangle]
pub unsafe extern "C" fn xmss_split_type_2_by_message(
    proof: *const u8,
    proof_len: usize,
    pubkeys: *const *const PublicKey,
    pubkey_counts: *const usize,
    count: usize,
    target_message: *const u8,
    log_inv_rate: usize,
    out: *mut u8,
    cap: usize,
    written: *mut usize,
) -> i32 {
    ffi_guard!(-1, {
        if proof.is_null() || proof_len == 0 || target_message.is_null() || written.is_null() {
            return -1;
        }
        let groups = match collect_key_groups(pubkeys, pubkey_counts, count) {
            Some(groups) => groups,
            None => return -1,
        };
        let proof = match MultiMessageAggregateSignature::decompress_without_pubkeys(
            slice::from_raw_parts(proof, proof_len),
            groups,
        ) {
            Some(proof) => proof,
            None => return -1,
        };
        let target: [u8; MESSAGE_LEN] =
            match slice::from_raw_parts(target_message, MESSAGE_LEN).try_into() {
                Ok(target) => target,
                Err(_) => return -1,
            };
        let proof = match std::panic::catch_unwind(AssertUnwindSafe(|| {
            split_multi_message_aggregate_by_message(proof, target, log_inv_rate)
        })) {
            Ok(Ok(proof)) => proof,
            _ => return -1,
        };
        write_out(&proof.compress_without_pubkeys(), out, cap, written)
    })
}

#[no_mangle]
pub unsafe extern "C" fn xmss_verify_type_2(
    proof: *const u8,
    proof_len: usize,
    pubkeys: *const *const PublicKey,
    pubkey_counts: *const usize,
    count: usize,
    message_hashes: *const u8,
    message_slots: *const u32,
) -> bool {
    ffi_guard!(false, {
        if proof.is_null() || proof_len == 0 || message_hashes.is_null() || message_slots.is_null()
        {
            return false;
        }
        let groups = match collect_key_groups(pubkeys, pubkey_counts, count) {
            Some(groups) => groups,
            None => return false,
        };
        let proof = match MultiMessageAggregateSignature::decompress_without_pubkeys(
            slice::from_raw_parts(proof, proof_len),
            groups,
        ) {
            Some(proof) => proof,
            None => return false,
        };
        if proof.info.len() != count {
            return false;
        }
        let hashes = slice::from_raw_parts(message_hashes, count * MESSAGE_LEN);
        let slots = slice::from_raw_parts(message_slots, count);
        for i in 0..count {
            let mut expected = [0; MESSAGE_LEN];
            expected.copy_from_slice(&hashes[i * MESSAGE_LEN..(i + 1) * MESSAGE_LEN]);
            if proof.info[i].without_pubkeys.message != expected
                || proof.info[i].without_pubkeys.slot != slots[i]
            {
                return false;
            }
        }
        verify_multi_message_aggregate(&proof).is_ok()
    })
}

// Poseidon permutation over KoalaBear, exposed for spec test vectors. The state
// is the canonical u32 field representation, permuted in place. This is the same
// instance the XMSS stack hashes with, so it matches the spec. Returns -1 on a
// null pointer or wrong width.
#[no_mangle]
pub unsafe extern "C" fn poseidon_permute_kb16(state: *mut u32, len: usize) -> i32 {
    ffi_guard!(-1, {
        if state.is_null() || len != 16 {
            return -1;
        }
        let raw = slice::from_raw_parts_mut(state, 16);
        let mut input = [0u32; 16];
        input.copy_from_slice(raw);
        let mut fe = KoalaBear::new_array(input);
        default_koalabear_poseidon1_16().permute_mut(&mut fe);
        for (dst, x) in raw.iter_mut().zip(fe.iter()) {
            *dst = x.as_canonical_u32();
        }
        0
    })
}

#[no_mangle]
pub unsafe extern "C" fn poseidon_permute_kb24(state: *mut u32, len: usize) -> i32 {
    ffi_guard!(-1, {
        if state.is_null() || len != 24 {
            return -1;
        }
        let raw = slice::from_raw_parts_mut(state, 24);
        let mut input = [0u32; 24];
        input.copy_from_slice(raw);
        let mut fe = KoalaBear::new_array(input);
        default_koalabear_poseidon1_24().permute_mut(&mut fe);
        for (dst, x) in raw.iter_mut().zip(fe.iter()) {
            *dst = x.as_canonical_u32();
        }
        0
    })
}
