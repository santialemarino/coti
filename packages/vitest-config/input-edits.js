import { createEvent, fireEvent } from '@testing-library/react';

/*
 * What a browser does to a text field on an edit, as far as jsdom can play it: `beforeinput` while
 * the field still holds its old text and selection, then the new text and caret, then `input`.
 * jsdom implements no `inputType`, so it is set on each event by hand.
 */
const setNativeValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;

function withInputType(event, inputType) {
  Object.defineProperty(event, 'inputType', { value: inputType });
  return event;
}

/* Fires `beforeinput` with the given type; returns whether the field cancelled it. */
export function beforeInput(field, inputType) {
  const event = withInputType(
    new Event('beforeinput', { bubbles: true, cancelable: true }),
    inputType,
  );
  fireEvent(field, event);
  return event.defaultPrevented;
}

/*
 * One edit: `raw` is the text the browser leaves and `caret` where it leaves the caret. Autofill
 * fires no `beforeinput` and carries no `inputType`; pass `inputType: null` for it.
 */
export function browserEdit(field, raw, { caret = raw.length, inputType = 'insertText' } = {}) {
  if (inputType !== null) beforeInput(field, inputType);
  setNativeValue.call(field, raw);
  field.setSelectionRange(caret, caret);
  const event = createEvent.input(field);
  if (inputType !== null) withInputType(event, inputType);
  fireEvent(field, event);
}

/* Each character typed at the caret, one keystroke at a time. */
export function typeText(field, text) {
  [...text].forEach((char) => {
    const start = field.selectionStart ?? field.value.length;
    const end = field.selectionEnd ?? start;
    browserEdit(field, field.value.slice(0, start) + char + field.value.slice(end), {
      caret: start + 1,
    });
  });
}
