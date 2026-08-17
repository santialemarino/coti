// The role that reaches the whole account. The API is still the authority — it answers 403
// on every admin route — so this only decides what the interface offers.
export const ADMIN_ROLE = 'ADMIN';

// A seller reaches the branches they are assigned to, and nothing else.
export const SELLER_ROLE = 'SELLER';

// Both values of the API's user_role enum, in the order the interface offers them.
export const USER_ROLES = [ADMIN_ROLE, SELLER_ROLE] as const;

export type UserRole = (typeof USER_ROLES)[number];
