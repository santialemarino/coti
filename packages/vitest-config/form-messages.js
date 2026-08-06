/*
 * Reading a form's error messages the way a user does, which is no longer the same as reading the
 * DOM. `FormMessage` holds its last body and fades it out with the collapsing box, so a message
 * that has left is still in the tree — what changes is that its box goes `aria-hidden`.
 *
 * `queryByText(...)` therefore cannot answer "is this message showing", and asserting it is null
 * would only pass by removing the exit animation again.
 */
const MESSAGE_SLOTS = '[data-slot="form-message"],[data-slot="form-root-message"]';

/* Every message currently offered to the user, in document order. */
export function shownMessages(container) {
  return [...container.querySelectorAll(MESSAGE_SLOTS)]
    .filter((box) => box.getAttribute('aria-hidden') !== 'true')
    .map((box) => box.textContent?.trim() ?? '');
}

export function isMessageShown(container, text) {
  return shownMessages(container).includes(text);
}

/*
 * Whether the words are still in the tree at all — true while a message is leaving, which is what
 * makes the collapse animate something rather than an empty box.
 */
export function isMessageHeld(container, text) {
  return [...container.querySelectorAll(MESSAGE_SLOTS)].some(
    (box) => box.textContent?.trim() === text,
  );
}
