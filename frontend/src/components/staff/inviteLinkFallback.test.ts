import { inviteLinkToShare } from "./inviteLinkFallback";

const url = "https://eat.example.com/staff/accept-invitation?token=t";

describe("inviteLinkToShare", () => {
  it("hands over the link when the send failed", () => {
    expect(
      inviteLinkToShare({ email_sent: false, invitation_url: url }, false),
    ).toBe(url);
  });

  it("hands over the link when the instance has email off, even if the log provider 'sent' it", () => {
    expect(
      inviteLinkToShare({ email_sent: true, invitation_url: url }, true),
    ).toBe(url);
  });

  it("stays quiet when the email really went out or there is no link", () => {
    expect(
      inviteLinkToShare({ email_sent: true, invitation_url: url }, false),
    ).toBeNull();
    expect(inviteLinkToShare({ email_sent: false }, true)).toBeNull();
  });
});
