// The switcher's stand-in for no selection at all, which the API reads as account-wide for an
// admin and the assigned set for a seller. An option needs a string value and no branch id can
// collide with this one.
export const ALL_BRANCHES = 'all';

// The window a branch may give its quotes, mirroring what the API accepts so a form can reject a
// value inline instead of turning it into a 400 with nothing to point at.
export const EXPIRY_MIN_DAYS = 1;
export const EXPIRY_MAX_DAYS = 365;

// A mirror of the API's BRANCH_DEFAULT_EXPIRY_DAYS, kept only so a new branch's form opens on the
// same number the API would have chosen. Tolerance to inflation differs by location, which is why
// the field is editable at all.
export const DEFAULT_EXPIRY_DAYS = 7;
