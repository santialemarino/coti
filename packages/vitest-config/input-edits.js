import { createEvent, fireEvent } from '@testing-library/react';

// A browser edit as jsdom can play it: `beforeinput` on the old text, the new text, then `input`.
// jsdom implements no `inputType`, so each event gets it by hand.
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

// One edit leaving `raw` and `caret`; pass `inputType: null` for autofill, which fires neither, and
// `announced: false` for an edit that skips `beforeinput`, like Chromium's scripted undo.
export function browserEdit(
  field,
  raw,
  { caret = raw.length, inputType = 'insertText', announced = inputType !== null } = {},
) {
  if (announced) beforeInput(field, inputType);
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
