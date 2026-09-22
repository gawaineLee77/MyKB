"""Self-contained rich HTML / Markdown with downloaded raster images."""

from __future__ import annotations

import base64
import html
import re
import urllib.parse
from html.parser import HTMLParser
from pathlib import Path
from typing import Any

from .common import ImportFailure, atomic_bytes, digest, private_dir
from .transport import S3Downloader


def image_type(data: bytes) -> str:
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        return "png"
    if data.startswith(b"\xff\xd8\xff"):
        return "jpeg"
    if data.startswith((b"GIF87a", b"GIF89a")):
        return "gif"
    if data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        return "webp"
    raise ImportFailure("image.unsupported_or_invalid")


def safe_link(value: str) -> str:
    try:
        parsed = urllib.parse.urlsplit(value)
        if parsed.scheme not in ("https", "http") or not parsed.hostname or parsed.username or parsed.password:
            return ""
        return value if not any(ord(char) < 32 for char in value) else ""
    except ValueError:
        return ""


class Images:
    def __init__(self, root: Path, downloader: S3Downloader, limits: dict[str, Any]):
        self.root, self.downloader = root, downloader
        self.limit = int(limits.get("max_image_mib", 20)) * 1024 * 1024
        self.total_limit = int(limits.get("max_embedded_images_mib", 100)) * 1024 * 1024
        self.max_count = int(limits.get("max_images_per_article", 200))
        self.total = 0
        self.calls = 0
        self.cache: dict[str, str] = {}
        self.sizes: dict[str, int] = {}
        self.records: list[dict[str, Any]] = []

    def embed(self, source: str) -> str:
        source = html.unescape(source).strip()
        key = digest(source.encode())
        self.calls += 1
        if self.calls > self.max_count:
            raise ImportFailure("image.count_limit")
        if key in self.cache:
            self.total += self.sizes[key]
            if self.total > self.total_limit:
                raise ImportFailure("image.byte_limit")
            return self.cache[key]
        private_dir(self.root / "images")
        if source.startswith("data:"):
            match = re.fullmatch(r"data:image/(?:png|jpeg|jpg|gif|webp);base64,([A-Za-z0-9+/=\s]+)", source)
            if not match or len(match[1]) > self.limit * 4 // 3 + 4096:
                raise ImportFailure("image.data_uri_invalid")
            try:
                data = base64.b64decode(re.sub(r"\s", "", match[1]), validate=True)
            except ValueError as exc:
                raise ImportFailure("image.data_uri_invalid") from exc
        else:
            temporary = self.root / "images" / (key + ".download")
            self.downloader.download(source, temporary, self.limit)
            data = temporary.read_bytes()
            temporary.unlink()
        if len(data) > self.limit or self.total + len(data) > self.total_limit:
            raise ImportFailure("image.byte_limit")
        kind = image_type(data)
        sha = digest(data)
        relative = f"images/{sha}.{kind}"
        atomic_bytes(self.root / relative, data)
        embedded = f"data:image/{kind};base64," + base64.b64encode(data).decode("ascii")
        self.cache[key] = embedded
        self.sizes[key] = len(data)
        self.total += len(data)
        self.records.append({"source_id": source if not source.startswith("data:") else "embedded:" + sha,
                             "path": relative, "sha256": sha, "bytes": len(data)})
        return embedded


class RichHTML(HTMLParser):
    """Keep useful static document structure, not scripts, events, or remote embeds."""

    allowed = {"p", "div", "span", "br", "hr", "h1", "h2", "h3", "h4", "h5", "h6", "strong", "b", "em", "i",
               "u", "s", "del", "ul", "ol", "li", "blockquote", "pre", "code", "table", "thead", "tbody", "tfoot",
               "tr", "td", "th", "caption", "figure", "figcaption", "a", "img", "sup", "sub"}
    blocked = {"script", "style", "iframe", "object", "embed", "form", "svg", "math", "noscript"}
    void = {"br", "hr", "img"}

    def __init__(self, images: Images):
        super().__init__(convert_charrefs=True)
        self.images, self.parts, self.suppressed = images, [], []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if self.suppressed:
            if tag in self.blocked and tag != "embed":
                self.suppressed.append(tag)
            return
        if tag in self.blocked:
            if tag != "embed":
                self.suppressed.append(tag)
            return
        if tag not in self.allowed:
            return
        attributes = dict(attrs)
        kept = {}
        if tag == "img":
            source = attributes.get("src") or ""
            if not source:
                raise ImportFailure("image.src_missing")
            kept = {"src": self.images.embed(source), "alt": attributes.get("alt") or ""}
        elif tag == "a":
            href = safe_link(attributes.get("href") or "")
            if href:
                kept["href"] = href
        elif tag in ("td", "th"):
            for name in ("rowspan", "colspan"):
                value = attributes.get(name) or ""
                if value.isdigit() and 1 <= int(value) <= 1000:
                    kept[name] = value
        self.parts.append("<" + tag + "".join(f' {name}="{html.escape(value, quote=True)}"' for name, value in kept.items()) + ">")

    def handle_endtag(self, tag: str) -> None:
        if self.suppressed:
            if tag in self.suppressed:
                while self.suppressed:
                    if self.suppressed.pop() == tag:
                        break
            return
        if tag in self.allowed and tag not in self.void:
            self.parts.append(f"</{tag}>")

    def handle_data(self, data: str) -> None:
        if not self.suppressed:
            self.parts.append(html.escape(data))


def rich_html(content: str, images: Images) -> str:
    parser = RichHTML(images)
    parser.feed(content)
    parser.close()
    return "".join(parser.parts)


def markdown_regions(content: str):
    """Separate code and escaped punctuation before interpreting live images."""
    def spans(text: str):
        start = index = 0
        token = re.compile(r"\\[!\"#$%&'()*+,\-./:;<=>?@\[\]\\^_`{|}~]|`+")
        while match := token.search(text, index):
            end = match.end()
            if match[0].startswith("`"):
                closing = re.search(r"(?<!`)" + re.escape(match[0]) + r"(?!`)", text[end:])
                if closing is None:
                    index = end
                    continue
                end += closing.end()
            if start < match.start():
                yield False, text[start:match.start()]
            yield True, text[match.start():end]
            start = index = end
        if start < len(text):
            yield False, text[start:]

    pending = []
    fence = ""
    for line in content.splitlines(keepends=True):
        marker = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line)
        if fence:
            yield True, line
            if marker and marker[1][0] == fence[0] and len(marker[1]) >= len(fence) and not marker[2].strip():
                fence = ""
        elif marker and (marker[1][0] != "`" or "`" not in marker[2]):
            yield from spans("".join(pending))
            pending = []
            fence = marker[1]
            yield True, line
        else:
            pending.append(line)
    yield from spans("".join(pending))


def markdown(content: str, images: Images) -> str:
    """Resolve inline/reference images outside code; do not fetch link targets."""
    regions = list(markdown_regions(content))
    references: dict[str, str] = {}
    for protected, text in regions:
        if not protected:
            for match in re.finditer(r'^ {0,3}\[([^]\n]+)\]:\s*<?([^\s>]+)>?(?:\s+["\'(].*)?$', text, re.MULTILINE):
                references.setdefault(match[1].strip().lower(), match[2])
    output = []
    for protected, text in regions:
        if protected:
            output.append(text)
            continue

        def inline(match: re.Match) -> str:
            return f"![{match[1]}]({images.embed(match[2] or match[3])})"

        def reference(match: re.Match) -> str:
            name = (match[2] or match[1]).strip().lower()
            if name not in references:
                raise ImportFailure("image.markdown_reference_missing")
            return f"![{match[1]}]({images.embed(references[name])})"

        def tag_image(match: re.Match) -> str:
            rendered = rich_html(match[0], images)
            image = re.fullmatch(r'<img src="([^"]*)" alt="([^"]*)">', rendered)
            if not image:
                raise ImportFailure("image.markdown_html_invalid")
            alt = html.unescape(image[2]).replace("]", r"\]")
            return f"![{alt}]({html.unescape(image[1])})"

        text = re.sub(r'!\[([^]\n]*)\]\(\s*(?:<([^>]+)>|([^\s)]+))(?:\s+["\'][^\n]*?["\'])?\s*\)', inline, text)
        text = re.sub(r"!\[([^]\n]*)\]\[([^]\n]*)\]", reference, text)
        text = re.sub(r"!\[([^]\n]*)\](?!\(|\[)", lambda match: reference_like_shortcut(match, references, images), text)
        text = re.sub(r"<img\b[^>]*>", tag_image, text, flags=re.IGNORECASE)
        remaining = re.sub(r"!\[[^\n]*?\]\(data:image/[^)]+\)", "", text)
        if re.search(r"!\[|<img\b", remaining, flags=re.IGNORECASE):
            raise ImportFailure("image.markdown_syntax_unsupported")
        output.append(text)
    return "".join(output)


def reference_like_shortcut(match: re.Match, references: dict[str, str], images: Images) -> str:
    key = match[1].strip().lower()
    if key not in references:
        raise ImportFailure("image.markdown_reference_missing")
    return f"![{match[1]}]({images.embed(references[key])})"


def metadata_html(article: dict[str, Any], attachments: list[dict[str, Any]]) -> str:
    escaped = html.escape
    parts = [f"<h1>{escaped(article['aritcleTitle'])}</h1>"]
    for label, key in (("文章标识", "articleCode"), ("最后修改时间", "lastUpdateDate"), ("摘要", "articleAbstract")):
        parts.append(f"<p>{label}：{escaped(str(article.get(key) or ''))}</p>")
    url = safe_link(article.get("kmsUrl") or "")
    if url:
        parts.append(f'<p>原文：<a href="{escaped(url, quote=True)}">{escaped(url)}</a></p>')
    if attachments:
        parts.append("<h2>附件清单</h2><ul>")
        parts.extend(f"<li>{escaped(item['filename'])}（关联文章：{escaped(article['articleCode'])}）</li>" for item in attachments)
        parts.append("</ul>")
    return "\n".join(parts)
