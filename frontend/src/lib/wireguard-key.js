// Generate a random WireGuard private key using Curve25519 clamping.
// The 32 random bytes come from the Web Crypto CSPRNG; bits are clamped so
// the scalar is a valid Curve25519 secret. The result is standard base64.
// Shared by ConfigEditor (new-config template) and FieldsEditor (the
// regenerate button in the edit view) so both paths use identical logic.
// 使用 Curve25519 钳制生成随机的 WireGuard 私钥。
// 32 个随机字节来自 Web Crypto 的密码学安全随机数发生器；按位钳制后即为
// 合法的 Curve25519 标量，结果为标准 base64。
// 由 ConfigEditor（新建模板）与 FieldsEditor（编辑视图的重新生成按钮）共用，
// 确保两条路径的算法完全一致。
export function generatePrivateKey() {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  bytes[0] &= 248;
  bytes[31] &= 127;
  bytes[31] |= 64;
  return btoa(String.fromCharCode(...bytes));
}
