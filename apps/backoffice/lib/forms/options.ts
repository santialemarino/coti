/*
 * When a field's message appears and when it changes, for every form in the app: nothing while the
 * caller is filling it the first time, then a re-check on each keystroke. react-hook-form gates the
 * second half on `isSubmitted`, which only `handleSubmit` sets — so a step gated with `trigger`
 * raises a message that then never updates.
 */
export const FORM_VALIDATION = {
  mode: 'onSubmit',
  reValidateMode: 'onChange',
} as const;
