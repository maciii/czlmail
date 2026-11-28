import DOMPurify from "dompurify";

export function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

/** 纯文本转成段落 HTML: 空行分段, 单个换行变 <br>。 */
export function textToHtml(text: string): string {
  const paras = text.replace(/\r\n/g, "\n").split(/\n{2,}/);
  return paras.map((p) => `<p>${escapeHtml(p).replace(/\n/g, "<br>")}</p>`).join("");
}

/**
 * 纯文本转成写信编辑器里的 HTML: 每个空行变成一个空段落, 与编辑器里按两次 Enter 的结果相同,
 * 段落本身没有上下间距(与 Outlook、Apple Mail、Gmail 一致)。AI 生成的文字、模板与
 * mailto 正文都走这里, 插入后看到的空行和预览里一样, 发出去也一样。
 */
export function textToEditorHtml(text: string): string {
  const paras = text.replace(/\r\n/g, "\n").split(/\n{2,}/);
  // 行首空格(缩进)换成 &nbsp;, 否则编辑器会把它们合并掉。
  const line = (l: string) => escapeHtml(l).replace(/^ +/, (sp) => "&nbsp;".repeat(sp.length));
  return paras.map((p) => `<p>${p.split("\n").map(line).join("<br>")}</p>`).join("<p></p>");
}

/**
 * 编辑器 HTML 转成发出去的正文。
 *
 * 编辑器里段落没有间距, 空段落占一行; 收件端却按浏览器默认给 <p> 上下各 1em, 而空的 <p></p>
 * 高度为 0 —— 结果是「每次回车都隔一大段、特意空出的行反而不见了」。发出去的邮件里只有内联样式
 * 可靠, 所以给段落写上 margin:0, 空段落填一个 &nbsp; (Outlook 的写法), 各家客户端显示一致。
 */
export function editorHtmlToEmail(html: string): string {
  if (!html) return html;
  const doc = new DOMParser().parseFromString(html, "text/html");
  doc.body.querySelectorAll("p").forEach((p) => {
    if (!/(^|;)\s*margin/i.test(p.getAttribute("style") ?? "")) {
      p.style.setProperty("margin", "0");
    }
    if (!p.textContent?.trim() && !p.querySelector("img, br")) p.innerHTML = "&nbsp;";
  });
  return doc.body.innerHTML;
}

/** 发出去的空段落(&nbsp;)在重新打开草稿时还原成编辑器的空段落。 */
export function emailHtmlToEditor(html: string): string {
  if (!html) return html;
  const doc = new DOMParser().parseFromString(html, "text/html");
  doc.body.querySelectorAll("p").forEach((p) => {
    if (p.childNodes.length === 1 && p.firstChild?.nodeType === Node.TEXT_NODE && p.textContent === "\u00a0") {
      p.textContent = "";
    }
  });
  return doc.body.innerHTML;
}

/** HTML 转纯文本, 用于生成邮件的 text/plain 部分与交给 AI。解析在惰性文档里进行。 */
export function htmlToText(html: string): string {
  if (!html) return "";
  const doc = new DOMParser().parseFromString(html, "text/html");
  doc.querySelectorAll("br").forEach((br) => br.replaceWith("\n"));
  doc.querySelectorAll("p, div, h1, h2, h3, li, tr, blockquote, pre").forEach((el) => el.append("\n"));
  doc.querySelectorAll("li").forEach((li) => li.prepend("• "));
  // 发出去的空段落是 &nbsp; (editorHtmlToEmail), 纯文本里还原成真正的空行。
  return (doc.body.textContent ?? "").replace(/^[ \t\u00a0]+$/gm, "").replace(/\n{3,}/g, "\n\n").trim();
}

/**
 * 清洗签名与引用这类要内联显示在应用里的 HTML。
 * 与邮件正文不同, 它们来自用户自己或已经显示过的邮件, 仍然去掉脚本、表单与事件处理器。
 */
export function sanitizeInline(html: string): string {
  return DOMPurify.sanitize(html, {
    FORBID_TAGS: ["script", "iframe", "object", "embed", "form", "input", "button", "style", "link", "meta", "base"],
    FORBID_ATTR: ["srcset", "ping", "formaction"],
  });
}

export function isHtmlEmpty(html: string): boolean {
  return htmlToText(html).trim() === "" && !/<img/i.test(html);
}

/**
 * 把本应用保存的草稿 HTML 拆回「正文 / 引用」。
 * 发送与存草稿时签名包在 div.signature 里、引用包在 div.czl-quote 里, 重新打开草稿时
 * 去掉签名(写信面板会按当前身份重新附上)并取出引用, 避免签名与引用叠加进编辑器。
 */
export function splitDraftHtml(html: string): { body: string; quote: string } {
  const doc = new DOMParser().parseFromString(html, "text/html");
  const quoteEl = doc.body.querySelector("div.czl-quote");
  const quote = quoteEl?.outerHTML ?? "";
  quoteEl?.remove();
  const sig = doc.body.querySelector("div.signature");
  if (sig) {
    const prev = sig.previousElementSibling;
    if (prev?.tagName === "P" && prev.textContent?.trim() === "--") prev.remove();
    sig.remove();
  }
  return { body: emailHtmlToEditor(doc.body.innerHTML), quote };
}
