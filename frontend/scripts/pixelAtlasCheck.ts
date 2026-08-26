// Task 13.3 — 校验 pixelAtlas 纯函数（布局数学 + 降级决策）。
// Run: node --experimental-strip-types scripts/pixelAtlasCheck.ts
import { layoutAtlas, decideRenderMode } from "../src/components/pixelAtlas.ts";

let failed = 0;
function assert(cond: boolean, msg: string) {
  if (cond) {
    console.log("ok  -", msg);
  } else {
    failed++;
    console.error("FAIL-", msg);
  }
}

// --- layoutAtlas ---
// 平方布局：4 帧 → 2×2 图集（64×64），非单行。
const l1 = layoutAtlas(4, 32, 32, 0);
assert(l1.width === 64 && l1.height === 64, "4×32² 图集 = 64×64（2×2 平方布局）");
assert(l1.rects.length === 4, "4 帧 4 个 rect");
assert(
  l1.rects.every((r) => r.w === 32 && r.h === 32) &&
    l1.rects[0].x === 0 && l1.rects[0].y === 0 &&
    l1.rects[1].x === 32 && l1.rects[1].y === 0 &&
    l1.rects[2].x === 0 && l1.rects[2].y === 32 &&
    l1.rects[3].x === 32 && l1.rects[3].y === 32,
  "rect 位置：行主序，整数像素",
);

const l2 = layoutAtlas(3, 32, 32, 4);
assert(l2.width === 68 && l2.height === 68, "3 帧含 padding：宽 68（2 列 × 2 行）");
assert(l2.rects[0].x === 0 && l2.rects[1].x === 36 && l2.rects[2].x === 0 && l2.rects[2].y === 36, "padding 参与布局（换行归零）");

const l0 = layoutAtlas(0, 32, 32, 0);
assert(l0.width === 0 && l0.height === 0 && l0.rects.length === 0, "空输入返回空布局");

let threw = false;
try {
  layoutAtlas(4, 0, 32, 0);
} catch {
  threw = true;
}
assert(threw, "非法 cell 尺寸抛错");

// --- decideRenderMode (降级缩放) ---
const m1 = decideRenderMode(32, 32, 4, 12);
assert(m1.scale === 12 && m1.renderGrid === true && m1.degraded === false, "常规 32²×4 帧：scale=12，网格开");

// 64×64×16 帧、base scale 16 → 1024×1024 超过 512×512 预算 → 降到 8；
// scale 8 ≥ minGridScale(8)，网格仍可读，保持开。
const m2 = decideRenderMode(64, 64, 16, 16);
assert(m2.scale === 8 && m2.degraded === true && m2.renderGrid === true, "大画布降级缩放：scale 16→8，degraded，网格保持");
assert(64 * 8 * 64 * 8 <= 512 * 512, "降级后舞台像素不超预算");

// 超大画布降到 scale=1：网格不可读，关闭。
const m3 = decideRenderMode(512, 512, 4, 16, { maxStagePixels: 128 * 128 });
assert(m3.scale === 1 && m3.renderGrid === false && m3.degraded === true, "512² 画布：scale=1，网格关");

// 多帧（>maxFrames）触发 degraded 标记（网格只看可读性，保持开）。
const m4 = decideRenderMode(32, 32, 100, 12, { maxFrames: 64 });
assert(m4.degraded === true && m4.renderGrid === true, "100 帧标记 degraded（网格仍可读）");

console.log(failed === 0 ? "\nALL PASS" : `\n${failed} FAILED`);
process.exit(failed === 0 ? 0 : 1);
