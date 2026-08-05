// The cap the API puts on every free-text field it stores, mirrored so a form rejects an
// over-long value inline instead of turning it into a 400 with nothing to point at.
export const TEXT_FIELD_MAX_LENGTH = 255;

// The wider cap the API keeps for a stored URL, which outgrows a name often enough to have its own.
export const URL_FIELD_MAX_LENGTH = 512;
