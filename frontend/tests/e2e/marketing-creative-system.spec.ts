import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { loginStaff } from "../helpers/staff-login";
import {
  API_BASE,
  MARKETING_BUSINESS_SLUG,
  MARKETING_MANAGER_EMAIL,
  journeysEnabled,
  marketingWriteDenyExists,
  resetMarketingJourneyFixture,
  resolveBusinessId,
  setMarketingWriteDeny,
} from "../helpers/journeys";

const CAPTION = "[E2E approval] Taste the moment at Demo AI Lounge.";

type CreativeSnapshot = {
  caption: string;
  image_url: string;
  image_source: string;
  template: string;
  aspect: string;
  slots: Record<string, string>;
  crop: { x: number; y: number; zoom: number };
  font_family: string;
};

type ActivityRow = {
  suggestion_id: string;
  title: string;
  status: string;
  creative_snapshot: CreativeSnapshot | null;
};

async function activityFor(
  api: APIRequestContext,
  auth: Record<string, string>,
  businessId: number,
  suggestionId: string,
): Promise<ActivityRow | undefined> {
  const response = await api.get(
    `${API_BASE}/inside/businesses/${businessId}/marketing/activity?per_page=100`,
    { headers: auth },
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  const body = (await response.json()) as { activity: ActivityRow[] };
  return body.activity.find((row) => row.suggestion_id === suggestionId);
}

async function applyExactCreative(page: Page): Promise<void> {
  const editor = page.getByRole("dialog").filter({
    has: page.getByText("Generate a photo, edit the text, download your post."),
  });
  await expect(editor).toBeVisible();

  await editor.getByRole("button", { name: "Story", exact: true }).click();
  await expect(
    editor.getByRole("button", { name: "Story", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await editor.getByRole("button", { name: "Bold", exact: true }).click();
  await expect(
    editor.getByRole("button", { name: "Bold", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");

  await editor.getByLabel("Horizontal focus").fill("0.2");
  await editor.getByLabel("Vertical focus").fill("0.8");
  await editor.getByLabel("Zoom").fill("1.4");
  await editor.getByLabel("Caption", { exact: true }).fill(CAPTION);

  await expect(editor.getByText("Menu photo · Free")).toBeVisible();
  await expect(
    editor.getByText("Ready to download", { exact: true }),
  ).toHaveAttribute("data-ready", "true");
}

async function copyCreative(page: Page): Promise<void> {
  const editor = page.getByRole("dialog").filter({
    has: page.getByText("Generate a photo, edit the text, download your post."),
  });
  await editor
    .getByRole("button", { name: "Copy caption", exact: true })
    .click();
  await expect(
    page.getByRole("dialog").filter({ hasText: "Was this post published?" }),
  ).toBeVisible();
}

test.describe("approval-first marketing creative system", () => {
  test.skip(
    !journeysEnabled,
    "Set PLAYWRIGHT_RUN_JOURNEYS_E2E=1 (requires full stack + demo seed)",
  );

  test("GM approves an exact creative, reuses it, pauses automation, and sees read-only controls", async ({
    context,
    page,
    playwright,
  }) => {
    test.setTimeout(120_000);
    const pageErrors: string[] = [];
    const consoleErrors: Array<{ text: string; url: string }> = [];
    const failedApiResponses: Array<{ status: number; url: string }> = [];
    const captionStatuses: number[] = [];
    let captionOutcome: "success" | "unavailable" | undefined;
    page.on("pageerror", (error) => pageErrors.push(error.message));
    page.on("console", (message) => {
      if (message.type() === "error") {
        consoleErrors.push({
          text: message.text(),
          url: message.location().url,
        });
      }
    });
    page.on("response", (response) => {
      if (!response.url().startsWith(API_BASE)) return;
      if (response.url().endsWith("/marketing/caption")) {
        captionStatuses.push(response.status());
      }
      if (response.status() >= 400) {
        failedApiResponses.push({
          status: response.status(),
          url: response.url(),
        });
      }
    });

    let api: APIRequestContext | undefined;
    let businessId: number | undefined;
    let auth: Record<string, string> = {};
    try {
      resetMarketingJourneyFixture(
        MARKETING_BUSINESS_SLUG,
        MARKETING_MANAGER_EMAIL,
        CAPTION,
      );
      expect(
        marketingWriteDenyExists(
          MARKETING_BUSINESS_SLUG,
          MARKETING_MANAGER_EMAIL,
        ),
        "self-healing setup must restore marketing:write",
      ).toBe(false);
      businessId = await resolveBusinessId(MARKETING_BUSINESS_SLUG);
      const journeyBusinessId = businessId;
      api = await playwright.request.newContext({ baseURL: API_BASE });
      const journeyApi = api;
      const token = await loginStaff(journeyApi, MARKETING_MANAGER_EMAIL);
      auth = { Authorization: `Bearer ${token}` };
      await context.addCookies((await journeyApi.storageState()).cookies);
      await context.grantPermissions(["clipboard-read", "clipboard-write"]);
      await page.addInitScript(() => {
        window.localStorage.setItem("payverge_had_session", "1");
        window.localStorage.setItem(
          "payverge_cookie_consent",
          JSON.stringify({
            version: 1,
            analytics: false,
            marketing: false,
            decidedAt: new Date().toISOString(),
          }),
        );
      });

      const settingsResponse = await journeyApi.get(
        `${API_BASE}/inside/businesses/${businessId}/marketing/settings`,
        { headers: auth },
      );
      expect(settingsResponse.ok(), await settingsResponse.text()).toBeTruthy();
      const enabledSettings = (await settingsResponse.json()) as {
        enabled: boolean;
      };
      expect(
        enabledSettings.enabled,
        "self-healing setup must enable suggestions",
      ).toBe(true);

      const suggestionsResponse = await journeyApi.get(
        `${API_BASE}/inside/businesses/${businessId}/marketing/suggestions`,
        { headers: auth },
      );
      expect(
        suggestionsResponse.ok(),
        await suggestionsResponse.text(),
      ).toBeTruthy();
      const suggestionsBody = (await suggestionsResponse.json()) as {
        suggestions: Array<{
          id: string;
          title: string;
          why_data: string;
          image_url?: string;
          image_source?: string;
        }>;
        paused: boolean;
      };
      expect(suggestionsBody.paused, "seeded suggestions must be enabled").toBe(
        false,
      );
      const suggestionCandidates = suggestionsBody.suggestions.filter(
        (item) =>
          item.image_url &&
          (!item.image_source || item.image_source === "menu"),
      );
      expect(
        suggestionCandidates.length,
        "seeded AI lounge must have menu suggestions with real images",
      ).toBeGreaterThan(0);

      await page.goto(`/business/${businessId}/dashboard`);
      const openNavigation = page.getByRole("button", {
        name: "Open navigation menu",
        exact: true,
      });
      if (await openNavigation.isVisible()) {
        await openNavigation.click();
      }
      await page
        .getByRole("button", { name: "Marketing", exact: true })
        .click();
      await expect(
        page.getByRole("heading", { name: "Marketing", exact: true }),
      ).toBeVisible({
        timeout: 20_000,
      });
      await expect(page.getByText("Live queue")).toBeVisible();
      await expect(
        page.getByRole("heading", { name: "Ready to post", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByText(/Your photo|Your offer|Your combo/).first(),
      ).toBeVisible();
      await expect(page.getByText("Credits", { exact: true })).toBeVisible();

      await expect
        .poll(
          () => {
            const unexpected = captionStatuses.find(
              (status) => (status < 200 || status >= 300) && status !== 503,
            );
            if (unexpected != null) return `unexpected:${unexpected}`;
            if (
              captionStatuses.some((status) => status >= 200 && status < 300)
            ) {
              return "success";
            }
            if (captionStatuses.includes(503)) return "unavailable";
            return "pending";
          },
          {
            message: "wait for the real caption provider response",
            timeout: 45_000,
          },
        )
        .toMatch(/^(success|unavailable)$/);
      captionOutcome = captionStatuses.some(
        (status) => status >= 200 && status < 300,
      )
        ? "success"
        : "unavailable";

      let suggestion: (typeof suggestionCandidates)[number] | undefined;
      if (captionOutcome === "success") {
        await expect
          .poll(
            async () => {
              let ready = 0;
              for (const candidate of suggestionCandidates) {
                ready += await page
                  .locator("article")
                  .filter({ hasText: candidate.why_data })
                  .getByText("Ready to post", { exact: true })
                  .count();
              }
              return ready;
            },
            {
              message: "wait for live caption readiness",
              timeout: 45_000,
            },
          )
          .toBeGreaterThan(0);
        for (const candidate of suggestionCandidates) {
          const candidateCard = page
            .locator("article")
            .filter({ hasText: candidate.why_data });
          if (
            (await candidateCard
              .getByText("Ready to post", { exact: true })
              .count()) > 0
          ) {
            suggestion = candidate;
            break;
          }
        }
      } else {
        await expect
          .poll(
            async () => {
              let unavailable = 0;
              for (const candidate of suggestionCandidates) {
                unavailable += await page
                  .locator("article")
                  .filter({ hasText: candidate.why_data })
                  .getByText("Couldn't generate a caption. Try again.")
                  .count();
              }
              return unavailable;
            },
            {
              message: "wait for the observed 503 fallback UI",
              timeout: 45_000,
            },
          )
          .toBeGreaterThan(0);
        for (const candidate of suggestionCandidates) {
          const candidateCard = page
            .locator("article")
            .filter({ hasText: candidate.why_data });
          if (
            (await candidateCard
              .getByText("Couldn't generate a caption. Try again.")
              .count()) > 0
          ) {
            suggestion = candidate;
            break;
          }
        }
      }
      expect(
        suggestion,
        "a menu suggestion must match the observed provider outcome",
      ).toBeTruthy();
      const card = page
        .locator("article")
        .filter({ hasText: suggestion!.why_data });
      await expect(card).toBeVisible();
      if (captionOutcome === "success") {
        await expect(
          card.getByText("Ready to post", { exact: true }),
        ).toBeVisible();
      } else {
        await expect(
          card.getByText("Couldn't generate a caption. Try again."),
        ).toBeVisible();
        await expect(
          card.getByText(/^Discover .+ at Demo AI Lounge\.$/),
        ).toBeVisible();
        await expect(
          card.getByRole("button", { name: "Try again", exact: true }),
        ).toBeVisible();
      }
      await expect(
        card.getByText("Menu signal", { exact: true }),
      ).toBeVisible();
      await card
        .getByRole("button", { name: "Review and export", exact: true })
        .click();
      await applyExactCreative(page);
      await copyCreative(page);
      await page.getByRole("button", { name: "Not yet", exact: true }).click();
      await expect(
        page
          .getByRole("dialog")
          .filter({ hasText: "Was this post published?" }),
      ).toHaveCount(0);
      await expect
        .poll(() =>
          activityFor(journeyApi, auth, journeyBusinessId, suggestion!.id),
        )
        .toBeUndefined();

      await card
        .getByRole("button", { name: "Review and export", exact: true })
        .click();
      await applyExactCreative(page);
      await copyCreative(page);
      const postRequestPromise = page.waitForRequest(
        (request) =>
          request
            .url()
            .endsWith(`/inside/businesses/${businessId}/marketing/activity`) &&
          request.method() === "POST",
      );
      await page
        .getByRole("button", { name: "Yes, it was published", exact: true })
        .click();
      const postRequest = await postRequestPromise;
      const postedPayload = postRequest.postDataJSON() as {
        action: string;
        creative_snapshot: CreativeSnapshot;
      };
      expect(postedPayload.action).toBe("post");
      expect(postedPayload.creative_snapshot).toMatchObject({
        caption: CAPTION,
        aspect: "9:16",
        template: "bold",
        crop: { x: 0.2, y: 0.8, zoom: 1.4 },
      });

      let posted: ActivityRow | undefined;
      await expect
        .poll(async () => {
          posted = await activityFor(
            journeyApi,
            auth,
            journeyBusinessId,
            suggestion!.id,
          );
          return posted?.status;
        })
        .toBe("posted");
      expect(posted?.creative_snapshot).toEqual(
        postedPayload.creative_snapshot,
      );

      const historyRow = page
        .locator("li")
        .filter({ hasText: CAPTION })
        .first();
      await expect(historyRow).toBeVisible();
      await expect(
        historyRow.getByText("Captured creative", { exact: true }),
      ).toBeVisible();
      await historyRow
        .getByRole("button", { name: "Reuse", exact: true })
        .click();
      const reusedEditor = page.getByRole("dialog").filter({
        has: page.getByText(
          "Generate a photo, edit the text, download your post.",
        ),
      });
      await expect(
        reusedEditor.getByLabel("Caption", { exact: true }),
      ).toHaveValue(CAPTION);
      await expect(
        reusedEditor.getByRole("button", { name: "Story", exact: true }),
      ).toHaveAttribute("aria-pressed", "true");
      await expect(
        reusedEditor.getByRole("button", { name: "Bold", exact: true }),
      ).toHaveAttribute("aria-pressed", "true");
      await expect(reusedEditor.getByLabel("Horizontal focus")).toHaveValue(
        "0.2",
      );
      await expect(reusedEditor.getByLabel("Vertical focus")).toHaveValue(
        "0.8",
      );
      await expect(reusedEditor.getByLabel("Zoom")).toHaveValue("1.4");
      await reusedEditor.getByText("Close", { exact: true }).click();

      await page
        .getByRole("button", { name: "Creative settings", exact: true })
        .click();
      const settingsDrawer = page.getByRole("dialog", {
        name: "Creative settings",
      });
      const prepareSuggestions = settingsDrawer.getByRole("switch", {
        name: "Prepare suggestions",
      });
      await expect(prepareSuggestions).toBeChecked();
      const pauseResponse = page.waitForResponse(
        (response) =>
          response
            .url()
            .endsWith(`/inside/businesses/${businessId}/marketing/settings`) &&
          response.request().method() === "PUT",
      );
      await prepareSuggestions.click();
      expect((await pauseResponse).ok()).toBeTruthy();
      await settingsDrawer
        .getByRole("button", { name: "Done", exact: true })
        .click();
      await expect(
        page.getByText("Suggestions are paused", { exact: true }),
      ).toBeVisible();

      const resumeResponse = page.waitForResponse(
        (response) =>
          response
            .url()
            .endsWith(`/inside/businesses/${businessId}/marketing/settings`) &&
          response.request().method() === "PUT",
      );
      await page
        .getByRole("button", { name: "Turn on auto-suggestions", exact: true })
        .click();
      expect((await resumeResponse).ok()).toBeTruthy();
      await expect(
        page.getByText("Suggestions are paused", { exact: true }),
      ).toHaveCount(0);

      setMarketingWriteDeny(
        MARKETING_BUSINESS_SLUG,
        MARKETING_MANAGER_EMAIL,
        true,
      );
      await page.reload();
      await expect(page.getByText(/View-only access:/)).toBeVisible({
        timeout: 20_000,
      });
      await expect(
        page.getByRole("button", { name: "Open studio", exact: true }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("button", { name: "Review and export", exact: true }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("button", { name: "Reuse", exact: true }),
      ).toHaveCount(0);
      await page
        .getByRole("button", { name: "Creative settings", exact: true })
        .click();
      const readOnlyDrawer = page.getByRole("dialog", {
        name: "Creative settings",
      });
      await expect(
        readOnlyDrawer.getByText(/role cannot change them/),
      ).toBeVisible();
      await expect(
        readOnlyDrawer.getByRole("switch", { name: "Prepare suggestions" }),
      ).toBeDisabled();

      const expectedCaption503 = (entry: { status?: number; url: string }) =>
        captionOutcome === "unavailable" &&
        captionStatuses.includes(503) &&
        entry.url.endsWith("/marketing/caption") &&
        (entry.status == null || entry.status === 503);
      expect(
        captionStatuses.every((status) =>
          captionOutcome === "success"
            ? status >= 200 && status < 300
            : status === 503,
        ),
        `caption responses must match observed ${captionOutcome} branch`,
      ).toBe(true);
      expect(pageErrors).toEqual([]);
      expect(
        failedApiResponses.filter((entry) => !expectedCaption503(entry)),
      ).toEqual([]);
      expect(
        consoleErrors.filter(
          (entry) =>
            !(
              expectedCaption503(entry) &&
              /Failed to load resource/.test(entry.text)
            ),
        ),
      ).toEqual([]);
    } finally {
      try {
        resetMarketingJourneyFixture(
          MARKETING_BUSINESS_SLUG,
          MARKETING_MANAGER_EMAIL,
          CAPTION,
        );
      } finally {
        await api?.dispose();
      }
    }
  });
});
