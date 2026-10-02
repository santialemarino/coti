import { describe, expect, it } from 'vitest';

import { anyBranchMissesEmail, missesBranchEmail } from '@/lib/utils/setup-issues';

describe('missesBranchEmail', () => {
  it.each([null, '', '   '])('reads %p as no mailbox', (email) => {
    expect(missesBranchEmail({ email })).toBe(true);
  });

  it('reads an address as a mailbox', () => {
    expect(missesBranchEmail({ email: 'moron@corralon.test' })).toBe(false);
  });
});

describe('anyBranchMissesEmail', () => {
  it('flags an open branch without a mailbox', () => {
    expect(
      anyBranchMissesEmail([
        { email: 'centro@corralon.test', isActive: true },
        { email: null, isActive: true },
      ]),
    ).toBe(true);
  });

  // A closed branch sends nothing, so its missing mailbox is not something to fix now.
  it('ignores a closed branch', () => {
    expect(
      anyBranchMissesEmail([
        { email: 'centro@corralon.test', isActive: true },
        { email: null, isActive: false },
      ]),
    ).toBe(false);
  });
});
