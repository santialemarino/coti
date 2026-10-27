// Administración's views, in the order the switch offers them; the first is the page's default.
export const ADMINISTRATION_VIEWS = ['history', 'pending'] as const;

export type AdministrationView = (typeof ADMINISTRATION_VIEWS)[number];
