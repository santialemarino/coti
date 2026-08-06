import { useEffect } from 'react';
import { render } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { describe, expect, it } from 'vitest';

import { Form, FormControl, FormField, FormItem, FormMessage, FormRootMessage } from './form';

const REJECTION = 'Ingresá tu correo electrónico.';

function messageBox(container: HTMLElement, slot: string) {
  const box = container.querySelector(`[data-slot="${slot}"]`);
  if (!box) throw new Error(`no ${slot} rendered`);
  return { hidden: box.getAttribute('aria-hidden') === 'true', text: box.textContent?.trim() };
}

/*
 * Driven through a real `useForm`, because what is under test is how the message reacts to an error
 * appearing and then leaving — which is react-hook-form's state, not a prop.
 */
function Harness({ error }: { error: boolean }) {
  const form = useForm({ defaultValues: { email: '' } });

  useEffect(() => {
    if (error) {
      form.setError('email', { message: REJECTION });
      form.setError('root', { message: REJECTION });
      return;
    }
    form.clearErrors();
  }, [error, form]);

  return (
    <Form {...form}>
      <FormField
        control={form.control}
        name="email"
        render={({ field }) => (
          <FormItem>
            <FormControl>
              <input {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormRootMessage />
    </Form>
  );
}

describe.each([
  ['form-message', 'FormMessage'],
  ['form-root-message', 'FormRootMessage'],
])('%s', (slot) => {
  it('offers the rejection while it stands', () => {
    const { container } = render(<Harness error />);

    expect(messageBox(container, slot)).toEqual({ hidden: false, text: REJECTION });
  });

  /*
   * The defect this pins: the body used to empty in the same commit the row started collapsing, so
   * the words vanished on frame one and an empty box animated its own height for 200ms. Holding the
   * last body is what gives the collapse something to take with it.
   */
  it('holds the rejection while the box collapses, and hides it from a screen reader', () => {
    const { container, rerender } = render(<Harness error />);

    rerender(<Harness error={false} />);

    expect(messageBox(container, slot)).toEqual({ hidden: true, text: REJECTION });
  });

  // Held, but not shown: the copy fades out with the box rather than surviving at full strength.
  it('fades the held rejection out', () => {
    const { container, rerender } = render(<Harness error />);
    const paragraph = container.querySelector(`[data-slot="${slot}"] p`);
    expect(paragraph?.className).not.toContain('opacity-0');

    rerender(<Harness error={false} />);

    expect(paragraph?.className).toContain('opacity-0');
    expect(paragraph?.className).toContain('transition-opacity');
  });

  it('renders nothing before anything has been rejected', () => {
    const { container } = render(<Harness error={false} />);

    expect(messageBox(container, slot)).toEqual({ hidden: true, text: '' });
  });
});
