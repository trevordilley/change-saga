import { createRequire } from "node:module";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { join } from "node:path";
import process from "node:process";

const inputPath = process.argv[2];
if (!inputPath) throw new Error("runner input path is required");
const input = JSON.parse(await readFile(inputPath, "utf8"));
const requireFromPlaywright = createRequire(join(input.playwright_dir, "package.json"));
let chromium;
try {
  ({ chromium } = requireFromPlaywright("playwright"));
} catch (error) {
  throw new Error(`cannot load Playwright from ${input.playwright_dir}; run npm ci there (${error.message})`);
}

const report = {
  version: 1,
  saga: input.saga,
  selection: input.selection,
  viewports: input.viewports,
  slides: [],
  contact_sheet: "contact-sheet.png",
  findings: [],
  passed: true,
  semantic_arrows: input.semantic_arrows
};
const browser = await chromium.launch({ headless: true });

function viewportName(viewport) {
  return `${viewport.width}x${viewport.height}`;
}

function finding(slide, surface, viewport, value) {
  report.findings.push({
    code: value.code,
    severity: value.severity || "error",
    surface,
    viewport: viewportName(viewport),
    deck: slide.deck,
    slide: slide.slide,
    ...(value.item ? { item: value.item } : {}),
    ...(value.other ? { other: value.other } : {}),
    message: value.message
  });
}

async function settle(page) {
  await page.evaluate(async () => {
    if (document.fonts?.ready) await document.fonts.ready;
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  });
}

async function inspectRaw(page, items) {
  return page.evaluate((authoredItems) => {
    const results = [];
    const viewport = { width: innerWidth, height: innerHeight };
    const rectangles = [];
    const overlapKinds = new Set(["node", "region", "statement", "risk", "metric", "example"]);
    for (const item of authoredItems) {
      if (item.selector?.type !== "element") continue;
      const element = document.getElementById(item.selector.element_id);
      if (!element) {
        results.push({ code: "missing_selector", item: item.id, message: `Item ${item.id} selects missing element #${item.selector.element_id}` });
        continue;
      }
      const rect = element.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) {
        results.push({ code: "empty_selector", item: item.id, message: `Item ${item.id} selects an element with no rendered bounds` });
        continue;
      }
      if (rect.left < -1 || rect.top < -1 || rect.right > viewport.width + 1 || rect.bottom > viewport.height + 1) {
        results.push({ code: "item_clipped", item: item.id, message: `Item ${item.id} extends outside the ${viewport.width}x${viewport.height} asset viewport` });
      }
      const tag = element.tagName.toLowerCase();
      const htmlBox = element.namespaceURI === "http://www.w3.org/1999/xhtml" && ["absolute", "fixed"].includes(getComputedStyle(element).position);
      const svgBox = element.namespaceURI === "http://www.w3.org/2000/svg" && ["rect", "image", "foreignobject"].includes(tag);
      if (overlapKinds.has(item.kind) && (htmlBox || svgBox)) rectangles.push({ id: item.id, element, rect: { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom, width: rect.width, height: rect.height } });
    }
    for (let i = 0; i < rectangles.length; i++) {
      for (let j = i + 1; j < rectangles.length; j++) {
        const left = Math.max(rectangles[i].rect.left, rectangles[j].rect.left);
        const top = Math.max(rectangles[i].rect.top, rectangles[j].rect.top);
        const right = Math.min(rectangles[i].rect.right, rectangles[j].rect.right);
        const bottom = Math.min(rectangles[i].rect.bottom, rectangles[j].rect.bottom);
        if (right <= left || bottom <= top) continue;
        const intersection = (right - left) * (bottom - top);
        const smaller = Math.min(rectangles[i].rect.width * rectangles[i].rect.height, rectangles[j].rect.width * rectangles[j].rect.height);
        const aContainsB = rectangles[i].element.contains(rectangles[j].element) || (rectangles[i].rect.left <= rectangles[j].rect.left + 1 && rectangles[i].rect.top <= rectangles[j].rect.top + 1 && rectangles[i].rect.right >= rectangles[j].rect.right - 1 && rectangles[i].rect.bottom >= rectangles[j].rect.bottom - 1);
        const bContainsA = rectangles[j].element.contains(rectangles[i].element) || (rectangles[j].rect.left <= rectangles[i].rect.left + 1 && rectangles[j].rect.top <= rectangles[i].rect.top + 1 && rectangles[j].rect.right >= rectangles[i].rect.right - 1 && rectangles[j].rect.bottom >= rectangles[i].rect.bottom - 1);
        if (!aContainsB && !bContainsA && smaller > 0 && intersection / smaller >= 0.15) {
          results.push({ code: "item_overlap", severity: "warning", item: rectangles[i].id, other: rectangles[j].id, message: `Addressable visual Items ${rectangles[i].id} and ${rectangles[j].id} overlap by at least 15% of the smaller bounds` });
        }
      }
    }
    if (document.documentElement.scrollWidth > viewport.width + 1 || document.documentElement.scrollHeight > viewport.height + 1) {
      results.push({ code: "asset_clipped", message: `Asset document exceeds the ${viewport.width}x${viewport.height} viewport` });
    }
    for (const element of document.querySelectorAll("body *:not(script):not(style), svg text, svg foreignObject")) {
      const value = (element.textContent || "").trim();
      if (!value || element.children.length > 0) continue;
      const style = getComputedStyle(element);
      if (style.display === "none" || style.visibility === "hidden") continue;
      const clipsX = style.overflowX === "hidden" || style.overflowX === "clip";
      const clipsY = style.overflowY === "hidden" || style.overflowY === "clip";
      if ((clipsX && element.scrollWidth > element.clientWidth + 1) || (clipsY && element.scrollHeight > element.clientHeight + 1)) {
        results.push({ code: "text_overflow", message: `Text is mechanically clipped in ${element.id ? `#${element.id}` : element.tagName.toLowerCase()}` });
      }
    }
    return results;
  }, items);
}

async function selectReviewerSlide(page, target) {
  const thumbnails = page.locator("[data-slide-thumbnail]");
  const count = await thumbnails.count();
  for (let i = 0; i < count; i++) {
    const candidate = thumbnails.nth(i);
    if (await candidate.getAttribute("data-slide-target") === target) {
      await candidate.click();
      return true;
    }
  }
  return false;
}

async function inspectReviewer(page, target) {
  return page.evaluate((wanted) => {
    const results = [];
    const slides = [...document.querySelectorAll("[data-deck-slide]")];
    const active = slides.find(slide => slide.dataset.slideTarget === wanted);
    if (!active) {
      results.push({ code: "missing_reviewer_slide", message: `Reviewer surface has no slide for ${wanted}` });
      return results;
    }
    if (active.hidden || getComputedStyle(active).display === "none") {
      results.push({ code: "hidden_reviewer_slide", message: `Reviewer did not activate ${wanted}` });
    }
    if (document.documentElement.scrollWidth > innerWidth + 1) {
      results.push({ code: "reviewer_horizontal_overflow", message: `Reviewer surface exceeds the ${innerWidth}px viewport width` });
    }
    const rect = active.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) {
      results.push({ code: "empty_reviewer_surface", message: `Reviewer slide ${wanted} has no rendered bounds` });
    }
    return results;
  }, target);
}

try {
  for (const slide of input.slides) {
    const slideReport = { target: slide.target, ...(slide.feature ? { feature: slide.feature } : {}), deck: slide.deck, slide: slide.slide, title: slide.title, artifacts: [] };
    const slideDir = join(input.output_dir, "slides", slide.deck, slide.slide);
    await mkdir(slideDir, { recursive: true });
    for (const viewport of input.viewports) {
      const dimensions = viewportName(viewport);
      const rawPath = join(slideDir, `raw-${dimensions}.png`);
      const rawPage = await browser.newPage({ viewport });
      const rawResponse = await rawPage.goto(input.base_url + slide.raw_url, { waitUntil: "networkidle" });
      if (!rawResponse?.ok()) throw new Error(`raw asset ${slide.raw_url} returned ${rawResponse?.status() ?? "no response"}`);
      await settle(rawPage);
      for (const value of await inspectRaw(rawPage, slide.items)) finding(slide, "raw", viewport, value);
      await rawPage.screenshot({ path: rawPath });
      await rawPage.close();
      slideReport.artifacts.push({ surface: "raw", viewport: dimensions, path: `slides/${slide.deck}/${slide.slide}/raw-${dimensions}.png` });

      const reviewerPath = join(slideDir, `reviewer-${dimensions}.png`);
      const reviewerPage = await browser.newPage({ viewport });
      const reviewerResponse = await reviewerPage.goto(input.base_url + slide.reviewer_url, { waitUntil: "networkidle" });
      if (!reviewerResponse?.ok()) throw new Error(`reviewer ${slide.reviewer_url} returned ${reviewerResponse?.status() ?? "no response"}`);
      await settle(reviewerPage);
      if (!await selectReviewerSlide(reviewerPage, slide.target)) {
        finding(slide, "reviewer", viewport, { code: "missing_reviewer_selector", message: `Reviewer navigation has no selector for ${slide.target}` });
      }
      await settle(reviewerPage);
      for (const value of await inspectReviewer(reviewerPage, slide.target)) finding(slide, "reviewer", viewport, value);
      await reviewerPage.screenshot({ path: reviewerPath });
      await reviewerPage.close();
      slideReport.artifacts.push({ surface: "reviewer", viewport: dimensions, path: `slides/${slide.deck}/${slide.slide}/reviewer-${dimensions}.png` });
    }
    report.slides.push(slideReport);
  }

  const cards = [];
  for (const slide of report.slides) {
    for (const artifact of slide.artifacts) {
      const bytes = await readFile(join(input.output_dir, artifact.path));
      cards.push(`<figure><img src="data:image/png;base64,${bytes.toString("base64")}" alt=""><figcaption>${escapeHTML(slide.deck)} / ${escapeHTML(slide.slide)} · ${artifact.surface} · ${artifact.viewport}</figcaption></figure>`);
    }
  }
  const contact = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  await contact.setContent(`<!doctype html><style>*{box-sizing:border-box}body{margin:0;padding:24px;background:#e9eef4;color:#172635;font:14px system-ui,sans-serif}h1{margin:0 0 18px;font-size:24px}.grid{display:grid;grid-template-columns:1fr 1fr;gap:18px}figure{margin:0;padding:10px;background:white;border:1px solid #bdc9d5;border-radius:8px}img{display:block;width:100%;aspect-ratio:16/9;object-fit:contain;background:#f7f9fc}figcaption{padding-top:8px;font-weight:600}</style><h1>Change Saga visual QA</h1><div class="grid">${cards.join("")}</div>`);
  await contact.screenshot({ path: join(input.output_dir, "contact-sheet.png"), fullPage: true });
  await contact.close();
  report.passed = !report.findings.some(value => value.severity === "error");
  await writeFile(join(input.output_dir, "visual-qa.json"), JSON.stringify(report, null, 2) + "\n");
} finally {
  await browser.close();
}

function escapeHTML(value) {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}
