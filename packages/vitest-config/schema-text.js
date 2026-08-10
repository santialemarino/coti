/*
 * The translator pair a form schema takes, as a double that reports which catalog a message came
 * from. A field message reads `field:<key>` and a shared one `shared:<key>`, so a test can assert
 * that "empty" and "malformed" resolve to different messages without knowing the Spanish.
 */
export function schemaText(prefixed = false) {
  const tag = (kind) => (key) => (prefixed ? `${kind}:${key}` : key);
  return { field: tag('field'), shared: tag('shared') };
}
