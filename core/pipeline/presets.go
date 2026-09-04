package pipeline

import (
	"fmt"
	"strings"
)

// StylePreset is a PerfectPixel generation style preset (阶段 3: PerfectPixel
// 四个风格预设). Each preset carries a deterministic English prompt fragment
// that is appended to the base filmstrip prompt so generation style is
// reproducible and auditable via the prompt snapshot.
type StylePreset struct {
	ID           string
	Name         string
	Description  string
	PromptSuffix string
}

// 契约文本来源：perfectpixel-studio internal/sprite/prompt.go（逐字对齐，勿改写）
const (
	ContractPixel = "true low-resolution pixel-art game sprite, like a 32-64px sprite enlarged on the canvas, " +
		"chunky readable silhouette, clean dark 1px outline, visible square pixel blocks, " +
		"grid-aligned hard pixel edges, limited shared palette, solid tone clusters, " +
		"flat color shading with at most one highlight step and one shadow step, " +
		"simple readable face and clearly separated limbs. " +
		"Never use painterly rendering, smooth gradients, airbrush shading, glossy lighting, " +
		"anti-aliased fine detail, high-definition pixel art, fine-grained pixel art, anime illustration, concept art, or 3D rendering."
	ContractChibi = "cute chibi game sprite with oversized head and small body, " +
		"bold dark outline, flat bright colors, minimal shading, large expressive eyes, " +
		"clean cartoon shapes readable at small size. " +
		"Never use realistic proportions, gradients, or painterly detail."
	ContractCartoon = "clean 2D cartoon game sprite, bold uniform outline, flat vivid colors, " +
		"simple two-tone cel shading, smooth rounded shapes, expressive but simple face. " +
		"Never use pixelation, gradients, photo textures, or 3D rendering."
	ContractRetro16 = "16-bit retro console era game sprite, restrained palette of 16-24 colors, " +
		"dark outline, dithering only where needed, compact proportions, " +
		"crisp hard pixel edges like a classic arcade fighter sprite. " +
		"Never use modern smooth shading or high-resolution detail."
)

var (
	StylePresetClassic = StylePreset{ID: "pixel", Name: "低分辨率像素", Description: "游戏精灵像素风：按 32-64px 低分辨率结构设计，强调清晰轮廓、硬边色块和简化细节；启用调色板量化，生成后最多保留 32 色", PromptSuffix: ContractPixel}
	StylePresetChibi   = StylePreset{ID: "chibi", Name: "Q版", Description: "Q版游戏精灵：大头小身、粗描边、平涂亮色和大眼睛；不启用调色板量化，保留生成颜色", PromptSuffix: ContractChibi}
	StylePresetCartoon = StylePreset{ID: "cartoon", Name: "卡通（非像素）", Description: "干净 2D 卡通：圆润造型、统一描边和双色赛璐璐明暗，明确不做像素化；不启用调色板量化，保留生成颜色", PromptSuffix: ContractCartoon}
	StylePresetRetro16 = StylePreset{ID: "retro16", Name: "复古 16-bit", Description: "复古 16-bit 游戏精灵：克制色板、深色描边和硬边造型；启用调色板量化，生成后最多保留 16 色", PromptSuffix: ContractRetro16}
)

func StylePresets() []StylePreset {
	return []StylePreset{StylePresetClassic, StylePresetChibi, StylePresetCartoon, StylePresetRetro16}
}

// PaletteSizeForStyle 对齐 perfectpixel pixelize.go：retro16→16，pixel→32，
// 其余（chibi/cartoon/custom）返回 0 表示跳过量化强制。
func PaletteSizeForStyle(styleID string) int {
	switch styleID {
	case "retro16":
		return 16
	case "pixel":
		return 32
	default:
		return 0
	}
}

// NegativePromptForStyle returns the built-in negative prompt portion of a style.
func NegativePromptForStyle(styleID string) string {
	style, err := StylePresetByID(styleID)
	if err != nil {
		return ""
	}
	const marker = "Never use "
	idx := strings.Index(style.PromptSuffix, marker)
	if idx < 0 {
		return ""
	}
	return style.PromptSuffix[idx:]
}

// StylePresetByID looks up a style preset by id.
func StylePresetByID(id string) (StylePreset, error) {
	for _, p := range StylePresets() {
		if p.ID == id {
			return p, nil
		}
	}
	return StylePreset{}, fmt.Errorf("pipeline: unknown style preset %q", id)
}

// ActionPreset is a motion action preset (动作预设) that supplies the
// deterministic animation instruction inside the filmstrip prompt.
type ActionPreset struct {
	ID           string
	Name         string
	Category     string // 分类：基础动作/战斗/受击状态/魔法技能/情感表达/交互
	Description  string // 展示给用户的简短说明（中文）
	Frames       int    // 建议帧数（0 → 前端按默认处理）
	Loop         bool   // 循环型动作（待机/行走等）vs 一次性动作（死亡/跳跃等）
	PromptText   string // 进入提示词的英文动作指令
	Choreography string // 进入提示词的逐帧编排细节（对齐 perfectpixel 的 MotionHint）
}

// 动作预设分类（与 perfectpixel 的动画情景分组对齐）。
const (
	ActionCategoryMovement = "基础动作"
	ActionCategoryCombat   = "战斗"
	ActionCategoryDamage   = "受击状态"
	ActionCategoryMagic    = "魔法技能"
	ActionCategoryEmotion  = "情感表达"
	ActionCategoryUtility  = "交互"
)

// Built-in action presets. 对标 perfectpixel-studio internal/sprite/presets.go
// 的常用动画情景（动作语义逐条精炼为提示词片段）。
var (
	ActionIdle   = ActionPreset{ID: "idle", Name: "待机", Category: ActionCategoryMovement, Description: "站立待机，轻微呼吸起伏", Frames: 4, Loop: true, PromptText: "idle animation, standing upright with a subtle breathing motion", Choreography: "Subtle in-place breathing cycle: gentle chest rise and fall, tiny up-down body shift of a few pixels, occasional blink. Feet stay planted in the same spot in every frame."}
	ActionWalk   = ActionPreset{ID: "walk", Name: "行走", Category: ActionCategoryMovement, Description: "行走循环，双脚交替迈步", Frames: 6, Loop: true, PromptText: "walking animation, steady stride cycle", Choreography: "Readable side-view walking cycle: alternating legs with clear contact and passing poses, opposite arm swing, slight body bob. Each frame shows a distinctly different leg position."}
	ActionRun    = ActionPreset{ID: "run", Name: "奔跑", Category: ActionCategoryMovement, Description: "快速奔跑循环，前倾冲刺", Frames: 6, Loop: true, PromptText: "running animation, fast stride cycle with strong body lean", Choreography: "Fast side-view running cycle: strong forward lean, large leg extension with airborne moments, pumping arms, pronounced body bob. Each frame is a distinct stride phase."}
	ActionJump   = ActionPreset{ID: "jump", Name: "跳跃", Category: ActionCategoryMovement, Description: "起跳-上升-下落-落地完整跳跃", Frames: 5, Loop: false, PromptText: "jump animation, crouch anticipation then leap upward, rise, fall, absorb landing", Choreography: "Jump sequence: crouching anticipation, take-off with body extended upward, airborne peak with legs tucked, landing recovery crouch. Vary the body's vertical position to show the arc."}
	ActionRoll   = ActionPreset{ID: "roll", Name: "翻滚", Category: ActionCategoryMovement, Description: "前滚翻：缩身团身翻滚一周后起身", Frames: 5, Loop: false, PromptText: "forward roll animation, tuck into a ball, rotate fully over the shoulder, rise back to standing", Choreography: "Evasive forward roll: tuck into a ball, rotate fully over the shoulder, and rise back to a crouch. Show clear rotation phases across frames."}
	ActionDash   = ActionPreset{ID: "dash", Name: "冲刺", Category: ActionCategoryMovement, Description: "快速冲刺：爆发起步向前疾冲", Frames: 4, Loop: false, PromptText: "dash animation, explosive crouch and push-off, body stretched low sprinting forward", Choreography: "Quick dash burst: explosive crouch-and-push start, body stretched low and forward at peak speed, then a brief settle. Strong horizontal lean throughout."}
	ActionCrouch = ActionPreset{ID: "crouch", Name: "下蹲", Category: ActionCategoryMovement, Description: "从站立屈膝下蹲并保持", Frames: 4, Loop: true, PromptText: "crouch animation, bend knees and lower into a compact crouch, hold still", Choreography: "Crouching sequence: from standing, bend knees and lower the body progressively into a compact crouch, head tucked slightly. Final frame is fully crouched."}
	ActionCrawl  = ActionPreset{ID: "crawl", Name: "爬行", Category: ActionCategoryMovement, Description: "手脚并用向前爬行", Frames: 6, Loop: true, PromptText: "crawl animation, hands-and-knees crawling cycle, alternating arm and opposite leg", Choreography: "Hands-and-knees crawling cycle: alternating arm-and-opposite-leg reaches, low body close to the ground, head up. Each frame a distinct crawl phase."}
	ActionClimb  = ActionPreset{ID: "climb", Name: "攀爬", Category: ActionCategoryMovement, Description: "沿垂直表面向上攀爬", Frames: 6, Loop: true, PromptText: "climb animation, hand-over-hand reaches with matching footholds climbing upward", Choreography: "Vertical climbing cycle: alternating hand-over-hand reaches and matching foot pushes, body pressed close to the surface, upward progress implied. Each frame a distinct reach."}
	ActionSwim   = ActionPreset{ID: "swim", Name: "游泳", Category: ActionCategoryMovement, Description: "游泳划水循环", Frames: 6, Loop: true, PromptText: "swim animation, alternating arm strokes with kicking legs", Choreography: "Swimming stroke cycle: arms reaching forward and pulling back in alternation, legs kicking, body horizontal. Each frame a distinct stroke phase."}
	ActionSit    = ActionPreset{ID: "sit", Name: "坐下", Category: ActionCategoryMovement, Description: "屈膝坐下到地面并放松", Frames: 4, Loop: false, PromptText: "sit animation, bend knees and hips, lower the body, settle onto the ground in a relaxed pose", Choreography: "Sitting down: bend at knees and hips, lower the body, settle onto the ground in a relaxed seated pose. Final frame clearly seated."}
	ActionSleep  = ActionPreset{ID: "sleep", Name: "睡觉", Category: ActionCategoryMovement, Description: "躺下睡觉，缓慢呼吸起伏", Frames: 4, Loop: true, PromptText: "sleep animation, lying down with eyes closed, slow gentle breathing rise and fall", Choreography: "Sleeping cycle: lying down with eyes closed, slow gentle breathing rise and fall, occasional small shift. Very calm, minimal motion."}
	ActionTurn   = ActionPreset{ID: "turn", Name: "转身", Category: ActionCategoryMovement, Description: "原地转身朝向另一侧", Frames: 4, Loop: false, PromptText: "turn animation, rotate the body from facing one way to the opposite direction", Choreography: "Turn-around: rotate the body from facing one way to the opposite, weight pivoting on the feet, head leading the turn. Show clear intermediate angles."}

	ActionAttack     = ActionPreset{ID: "attack", Name: "攻击", Category: ActionCategoryCombat, Description: "单次攻击：蓄力-挥击-收势", Frames: 5, Loop: false, PromptText: "attack animation, one quick strike with anticipation wind-up and recovery", Choreography: "Melee attack: wind-up with body coiled back, powerful strike at full extension, follow-through, recovery to ready stance. The strike frame is the most extreme pose."}
	ActionSlash      = ActionPreset{ID: "slash", Name: "劈砍", Category: ActionCategoryCombat, Description: "横向挥砍/劈斩武器攻击", Frames: 5, Loop: false, PromptText: "slash animation, coil the blade back then sweep it across in a wide horizontal arc at full extension", Choreography: "Sword slash: coil the blade back, sweep it across in a wide horizontal arc at full extension, follow through to the opposite side, recover. Most extreme pose mid-swing."}
	ActionStab       = ActionPreset{ID: "stab", Name: "突刺", Category: ActionCategoryCombat, Description: "向前突刺攻击", Frames: 4, Loop: false, PromptText: "thrust animation, draw the weapon back close then explosive straight forward lunge", Choreography: "Thrust attack: draw the weapon back close to the body, explosive straight forward lunge with full arm and weapon extension, then retract. Peak frame fully extended forward."}
	ActionPunch      = ActionPreset{ID: "punch", Name: "拳击", Category: ActionCategoryCombat, Description: "直拳击出", Frames: 4, Loop: false, PromptText: "punch animation, cock the fist back at the hip, drive it forward with shoulder rotation to full extension", Choreography: "Straight punch: cock the fist back at the hip, drive it forward with shoulder rotation to full extension, retract to guard. Peak frame fully extended."}
	ActionKick       = ActionPreset{ID: "kick", Name: "踢击", Category: ActionCategoryCombat, Description: "高踢腿攻击", Frames: 5, Loop: false, PromptText: "kick animation, chamber the knee then snap the leg out to full extension, hold impact pose, retract", Choreography: "High kick: plant and chamber the knee, snap the leg out to full extension, hold the impact pose, retract and settle. Peak frame at maximum leg extension."}
	ActionBlock      = ActionPreset{ID: "block", Name: "格挡", Category: ActionCategoryCombat, Description: "举武器/盾防御格挡", Frames: 3, Loop: true, PromptText: "block animation, raise guard to defend, brace with slight crouch, hold the defensive pose", Choreography: "Defensive block: raise arms or shield to guard, brace with a slight crouch, hold firm with tiny tension shifts. Feet planted, posture steady."}
	ActionDodge      = ActionPreset{ID: "dodge", Name: "闪避", Category: ActionCategoryCombat, Description: "快速侧移闪避攻击", Frames: 4, Loop: false, PromptText: "dodge animation, fast lean-and-step to one side evading, body weaves out of the way, then recovers", Choreography: "Dodge: a fast lean-and-step to one side to evade, body weaving out of the way, then recovering balance. Quick lateral motion."}
	ActionBackstep   = ActionPreset{ID: "backstep", Name: "后撤步", Category: ActionCategoryCombat, Description: "快速向后跳步拉开距离", Frames: 4, Loop: false, PromptText: "backstep animation, quick defensive hop backward with light push-off and brief airborne moment", Choreography: "Backstep: a quick defensive hop backward, light push off the front foot, brief airborne drift, land back in guard. Net backward movement."}
	ActionShoot      = ActionPreset{ID: "shoot", Name: "射击", Category: ActionCategoryCombat, Description: "远程武器射击并后坐", Frames: 4, Loop: false, PromptText: "shoot animation, steady the weapon, fire with sharp recoil kick pushing the body back", Choreography: "Ranged shot: steady the weapon, fire with a sharp recoil kick pushing the body back, then settle back on target. Show the recoil clearly. No projectile particles separated from the weapon."}
	ActionThrow      = ActionPreset{ID: "throw", Name: "投掷", Category: ActionCategoryCombat, Description: "过肩投掷物体", Frames: 5, Loop: false, PromptText: "throw animation, wind the arm back overhead, whip it forward releasing at full extension", Choreography: "Overhand throw: wind the arm back behind the head, whip it forward releasing at full extension, follow through across the body. Peak frame at release."}
	ActionCombo      = ActionPreset{ID: "combo", Name: "连击", Category: ActionCategoryCombat, Description: "多段快速连续攻击", Frames: 6, Loop: false, PromptText: "combo animation, fast sequence of distinct strikes from different angles, slash then follow-up hit", Choreography: "Multi-hit combo: a fast sequence of distinct strikes from different angles (e.g. slash, backslash, thrust), each frame a separate hit, ending in a recovery pose."}
	ActionPowerUp    = ActionPreset{ID: "power-up", Name: "蓄力爆发", Category: ActionCategoryCombat, Description: "蓄力聚集后爆发攻击", Frames: 6, Loop: false, PromptText: "power-up animation, held loading pose gathering strength, body tense, then release a powerful attack", Choreography: "Power-up loop: braced wide stance, fists clenched, body trembling with effort and energy surging upward, hair and clothes lifting. Looping intensity, feet planted."}
	ActionSpinAttack = ActionPreset{ID: "spin-attack", Name: "回旋斩", Category: ActionCategoryCombat, Description: "原地旋转一周挥击", Frames: 6, Loop: false, PromptText: "spin attack animation, rotate the whole body a full turn while sweeping the weapon around in a circle", Choreography: "Spin attack: rotate the whole body a full turn while sweeping the weapon around in a wide circle, then settle facing forward. Show distinct rotation angles per frame."}
	ActionTaunt      = ActionPreset{ID: "taunt", Name: "挑衅", Category: ActionCategoryCombat, Description: "嘲讽手势挑衅对手", Frames: 4, Loop: true, PromptText: "taunt animation, confident provoking gesture, beckoning with a hand, chest puffed, bobbing in place", Choreography: "Taunt: a confident provoking gesture — beckoning with a hand, chest puffed, head cocked — looping with attitude. Feet planted, upper body expressive."}

	ActionHurt      = ActionPreset{ID: "hurt", Name: "受击", Category: ActionCategoryDamage, Description: "被击打后向后踉跄", Frames: 3, Loop: false, PromptText: "hurt animation, body recoils backward, head snaps back, brief stagger with arms flailing", Choreography: "Hit reaction: body recoils backward, head snaps back, brief stagger with arms flailing slightly, then a weakened guard pose. Feet roughly in place."}
	ActionKnockback = ActionPreset{ID: "knockback", Name: "击退", Category: ActionCategoryDamage, Description: "被重击向后击飞", Frames: 4, Loop: false, PromptText: "knockback animation, launched backward off the feet from a blow, body airborne briefly", Choreography: "Knockback: launched backward off the feet from a blow, body airborne and tumbling backward, then a hard skidding stop. Clear backward travel through the air."}
	ActionKnockdown = ActionPreset{ID: "knockdown", Name: "击倒", Category: ActionCategoryDamage, Description: "被击中失去平衡倒地", Frames: 4, Loop: false, PromptText: "knockdown animation, struck and losing footing, body rotates and drops, landing flat on the ground", Choreography: "Knockdown: struck and losing footing, body rotates and drops, landing flat on the back or side on the ground. Final frame fully down."}
	ActionGetUp     = ActionPreset{ID: "get-up", Name: "起身", Category: ActionCategoryDamage, Description: "从倒地状态撑起站立", Frames: 5, Loop: false, PromptText: "get-up animation, from lying on the ground push up with the arms, draw the legs under, rise to standing", Choreography: "Get up: from lying on the ground, push up with the arms, draw the legs under, rise through a crouch back to standing. Clear upward progression."}
	ActionStun      = ActionPreset{ID: "stun", Name: "眩晕", Category: ActionCategoryDamage, Description: "被眩晕原地摇晃", Frames: 4, Loop: true, PromptText: "stun animation, dazed slumped posture, head lolling, body swaying off balance", Choreography: "Stunned loop: dazed slumped posture, head lolling, body swaying off balance, knees buckling slightly. Looping wobble, feet barely holding."}
	ActionDeath     = ActionPreset{ID: "death", Name: "死亡", Category: ActionCategoryDamage, Description: "中招-倒地-躺平的死亡过程", Frames: 5, Loop: false, PromptText: "death animation, stagger, collapse to the knees, fall further down, lie flat on the ground", Choreography: "Defeat sequence: stagger, collapse to the knees, fall further down, finally lying flat on the ground. Final frame clearly lying down."}
	ActionDeathFall = ActionPreset{ID: "death-fall", Name: "坠落死亡", Category: ActionCategoryDamage, Description: "向后倒下的死亡动画", Frames: 4, Loop: false, PromptText: "death-fall animation, thrown backward, arms flung out, body arcs back and drops to the ground", Choreography: "Falling death: thrown backward, arms flung out, body arcing back and dropping, landing flat and motionless. Final frame fully down and still."}
	ActionRevive    = ActionPreset{ID: "revive", Name: "复活", Category: ActionCategoryDamage, Description: "从躺平状态挣扎起身复活", Frames: 6, Loop: false, PromptText: "revive animation, from lying flat the body stirs, lifts, and rises back to life through a kneel", Choreography: "Revive: from lying flat, the body stirs, lifts, and rises through a kneeling pose back to a strong standing stance, head lifting last. Gradual return of strength."}
	ActionLowHP     = ActionPreset{ID: "low-hp", Name: "残血", Category: ActionCategoryDamage, Description: "濒死虚弱弓身喘息", Frames: 4, Loop: true, PromptText: "low-hp animation, hunched and exhausted, one hand braced on a knee, heavy labored breathing", Choreography: "Low-HP loop: hunched and exhausted, one hand braced on a knee, heavy labored breathing, slight unsteady sway. Barely standing, looping fatigue."}

	ActionCast     = ActionPreset{ID: "cast", Name: "施法", Category: ActionCategoryMagic, Description: "通用法术吟唱施放", Frames: 5, Loop: false, PromptText: "spell cast animation, arms gather inward in concentration, then thrust forward in a casting motion", Choreography: "Spell casting: arms gather inward in concentration, then thrust forward in a casting pose, followed by recovery. Pose changes only, no floating magical particles."}
	ActionCastFire = ActionPreset{ID: "cast-fire", Name: "火焰法术", Category: ActionCategoryMagic, Description: "火焰魔法施放", Frames: 6, Loop: false, PromptText: "fire spell animation, gather energy at the hands with a coiled stance, then thrust both hands forward releasing flames", Choreography: "Fire spell cast: gather energy at the hands with a coiled stance, then thrust both hands forward releasing the blast. Any flame must be opaque, hard-edged, and touching the hands, not floating particles."}
	ActionCastIce  = ActionPreset{ID: "cast-ice", Name: "寒冰法术", Category: ActionCategoryMagic, Description: "寒冰魔法施放", Frames: 6, Loop: false, PromptText: "ice spell animation, slow controlled gathering pose, hands sweep inward, then a sharp outward release of ice", Choreography: "Ice spell cast: a slow controlled gathering pose, hands sweeping inward, then a sharp pointed release forward. Cold, precise, deliberate motion."}
	ActionCastHeal = ActionPreset{ID: "cast-heal", Name: "治疗法术", Category: ActionCategoryMagic, Description: "对自身施放治疗", Frames: 5, Loop: false, PromptText: "healing spell animation, bring hands together at the chest in a gentle gathering pose, warm glow radiating outward", Choreography: "Healing cast: bring hands together at the chest in a gentle gathering pose, then open them outward and upward in a soft release, head tilted up. Calm, flowing motion."}
	ActionChannel  = ActionPreset{ID: "channel", Name: "引导", Category: ActionCategoryMagic, Description: "持续引导聚集能量", Frames: 4, Loop: true, PromptText: "channel animation, sustained focused pose, hands held out gathering energy", Choreography: "Channeling loop: a sustained focused pose, hands held out gathering energy, body tense with small pulsing shifts and a slight glow at the hands. Looping concentration."}
	ActionShield   = ActionPreset{ID: "shield", Name: "护盾", Category: ActionCategoryMagic, Description: "架起魔法护盾屏障", Frames: 4, Loop: false, PromptText: "shield animation, sweep one arm forward and out to project a magical barrier, body braced behind it", Choreography: "Shield up: sweep one arm forward and out to project a barrier, body braced behind it, feet planted wide. The barrier shimmers into place and holds steady until the settle."}
	ActionSummon   = ActionPreset{ID: "summon", Name: "召唤", Category: ActionCategoryMagic, Description: "高举双臂施展召唤", Frames: 5, Loop: false, PromptText: "summon animation, crouch and gather low, then rise sweeping both arms upward and outward in an invoking gesture", Choreography: "Summon: crouch and gather low, then rise sweeping both arms upward and outward in a grand calling gesture, finishing tall with arms raised. Build to the peak."}
	ActionMeditate = ActionPreset{ID: "meditate", Name: "冥想", Category: ActionCategoryMagic, Description: "盘坐冥想静心", Frames: 4, Loop: true, PromptText: "meditate animation, seated cross-legged, hands resting on knees, eyes closed, calm steady breathing", Choreography: "Meditation loop: seated cross-legged, hands resting on knees, eyes closed, very slow calm breathing rise and fall. Minimal serene motion."}

	ActionWave      = ActionPreset{ID: "wave", Name: "挥手", Category: ActionCategoryEmotion, Description: "友好挥手打招呼", Frames: 4, Loop: true, PromptText: "wave animation, friendly greeting, one arm raises and waves side to side while the body stays still", Choreography: "Friendly greeting: one arm raises and waves side to side across frames while the rest of the body stays still. Hand in clearly different positions each frame. Feet planted."}
	ActionCheer     = ActionPreset{ID: "cheer", Name: "欢呼", Category: ActionCategoryEmotion, Description: "高举双臂欢呼雀跃", Frames: 4, Loop: true, PromptText: "cheer animation, throw both arms up overhead repeatedly with a small hop, happy and energetic", Choreography: "Cheering loop: throw both arms up overhead repeatedly with a small hop or bounce, head up, joyful. Energetic looping celebration."}
	ActionBow       = ActionPreset{ID: "bow", Name: "鞠躬", Category: ActionCategoryEmotion, Description: "恭敬弯腰鞠躬", Frames: 4, Loop: false, PromptText: "bow animation, from standing bend forward at the waist into a respectful bow, hold briefly, then rise back up", Choreography: "Bow: from standing, bend forward at the waist into a respectful bow, hold briefly, then rise back up. Show the full forward bend."}
	ActionNod       = ActionPreset{ID: "nod", Name: "点头", Category: ActionCategoryEmotion, Description: "点头表示同意", Frames: 3, Loop: false, PromptText: "nod animation, tip the head down and back up in agreement, small body settle", Choreography: "Nod: tip the head down and back up in agreement, small body settle. Head clearly moves down then up. Body otherwise still."}
	ActionDance     = ActionPreset{ID: "dance", Name: "跳舞", Category: ActionCategoryEmotion, Description: "节奏感全身舞动", Frames: 6, Loop: true, PromptText: "dance animation, rhythmic full-body movement, hips and arms swaying, weight shifting foot to foot", Choreography: "Dancing loop: rhythmic full-body movement — hips and arms swaying, weight shifting foot to foot, head bobbing to a beat. Distinct fun poses per frame, looping smoothly."}
	ActionVictory   = ActionPreset{ID: "victory", Name: "胜利", Category: ActionCategoryEmotion, Description: "胜利庆祝姿势", Frames: 4, Loop: true, PromptText: "victory animation, triumphant pose, fist pumped or arms raised, chest out, small celebratory bounce", Choreography: "Victory loop: a triumphant pose — fist pumped or arms raised, chest out, small confident bounce. Looping celebration, proud and energetic."}
	ActionLaugh     = ActionPreset{ID: "laugh", Name: "大笑", Category: ActionCategoryEmotion, Description: "开心大笑前仰后合", Frames: 4, Loop: true, PromptText: "laugh animation, head tipped back, shoulders bouncing with laughter, one hand to the belly", Choreography: "Laughing loop: head tipped back, shoulders bouncing with laughter, maybe a hand to the belly, big smile. Looping bounce of joy."}
	ActionCry       = ActionPreset{ID: "cry", Name: "哭泣", Category: ActionCategoryEmotion, Description: "掩面啜泣", Frames: 4, Loop: true, PromptText: "cry animation, hands toward the face, shoulders shaking with sobs, head bowed, body hunched", Choreography: "Crying loop: hands toward the face, shoulders shaking with sobs, head bowed, body hunched. Looping sad tremble. Tears optional but must be small and on the face."}
	ActionAngry     = ActionPreset{ID: "angry", Name: "愤怒", Category: ActionCategoryEmotion, Description: "握拳颤抖发怒", Frames: 4, Loop: true, PromptText: "angry animation, fists clenched, shoulders raised and tense, body trembling with rage", Choreography: "Angry loop: fists clenched, shoulders raised and tense, body trembling with rage, leaning forward, brows down. Looping fury, feet planted."}
	ActionSurprised = ActionPreset{ID: "surprised", Name: "惊讶", Category: ActionCategoryEmotion, Description: "受惊弹起后退", Frames: 3, Loop: false, PromptText: "surprised animation, sharp startled jolt, body snaps upright and back, arms fly up", Choreography: "Surprise: a sharp startled jolt — body snaps upright and back, arms fly up, head rears, eyes wide. Quick recoil then a frozen shocked pose."}
	ActionSad       = ActionPreset{ID: "sad", Name: "难过", Category: ActionCategoryEmotion, Description: "垂头丧气低落", Frames: 4, Loop: true, PromptText: "sad animation, shoulders slumped, head down, arms hanging limp, slow heavy sway", Choreography: "Sad loop: shoulders slumped, head down, arms hanging limp, a slow heavy sway and sigh. Looping melancholy, minimal motion."}
	ActionPoint     = ActionPreset{ID: "point", Name: "指向", Category: ActionCategoryEmotion, Description: "伸手指向前方", Frames: 4, Loop: false, PromptText: "point animation, draw the arm back then thrust it forward extending one finger to point decisively", Choreography: "Pointing: draw the arm back then thrust it forward extending one finger to point decisively, body leaning into it, then hold. Peak frame fully extended forward."}
	ActionSalute    = ActionPreset{ID: "salute", Name: "敬礼", Category: ActionCategoryEmotion, Description: "军礼致敬", Frames: 4, Loop: false, PromptText: "salute animation, snap one hand up to the brow in a crisp salute, body straightening to attention", Choreography: "Salute: snap one hand up to the brow in a crisp military salute, body straightening to attention, hold, then lower. Sharp and formal."}

	ActionPickUp = ActionPreset{ID: "pick-up", Name: "拾取", Category: ActionCategoryUtility, Description: "弯腰拾起地上物品", Frames: 4, Loop: false, PromptText: "pick-up animation, bend at the knees and waist down toward the ground, close the hand around an item, rise back up", Choreography: "Pick up: bend at the knees and waist down toward the ground, close the hand as if grasping an item, then rise back up holding it. Clear down-then-up motion."}
	ActionPush   = ActionPreset{ID: "push", Name: "推", Category: ActionCategoryUtility, Description: "用力推动重物", Frames: 6, Loop: true, PromptText: "push animation, lean hard forward with both arms extended against an object, legs driving", Choreography: "Pushing loop: leaning hard forward with both arms extended against an object, legs driving with alternating steps, straining. Looping effortful push."}
	ActionPull   = ActionPreset{ID: "pull", Name: "拉", Category: ActionCategoryUtility, Description: "向后拉动重物", Frames: 6, Loop: true, PromptText: "pull animation, lean back with both arms gripping something, legs stepping backward", Choreography: "Pulling loop: leaning back with both arms drawn in gripping something, legs stepping backward and digging in, straining. Looping effortful pull."}
	ActionOpen   = ActionPreset{ID: "open", Name: "开门", Category: ActionCategoryUtility, Description: "伸手开门或开箱", Frames: 4, Loop: false, PromptText: "open animation, reach forward toward a handle, grip and pull or push it open with a turning motion", Choreography: "Open: reach forward toward a handle, grip and pull or push it open with a turning motion, lean in. Clear reach-and-open action with the arm doing the work."}
	ActionEat    = ActionPreset{ID: "eat", Name: "进食", Category: ActionCategoryUtility, Description: "抬手送食咀嚼", Frames: 4, Loop: false, PromptText: "eat animation, raise a hand to the mouth as if holding food, take a bite with a small head tilt, chew", Choreography: "Eating: raise a hand to the mouth as if holding food, take a bite with a small head tilt, lower the hand, chew. Clear hand-to-mouth motion."}
	ActionDrink  = ActionPreset{ID: "drink", Name: "饮水", Category: ActionCategoryUtility, Description: "举杯仰头喝水", Frames: 4, Loop: false, PromptText: "drink animation, raise a hand to the mouth as if holding a cup, tip the head back to drink", Choreography: "Drinking: raise a hand to the mouth as if holding a cup, tip the head back to drink, then lower. Clear raise-tip-lower motion."}
	ActionDig    = ActionPreset{ID: "dig", Name: "挖掘", Category: ActionCategoryUtility, Description: "挥锹挖土循环", Frames: 6, Loop: true, PromptText: "dig animation, thrust a shovel down into the ground, scoop, lift and toss the dirt aside", Choreography: "Digging loop: thrust a shovel down into the ground, scoop, lift and toss the dirt aside, return. Looping dig cycle with clear down-scoop-toss phases."}

	// 2025-11 增补的常用动画（对标 perfectpixel 常用战斗/受击/交互情景）。
	ActionHeavyAttack = ActionPreset{ID: "attack-heavy", Name: "重击", Category: ActionCategoryCombat, Description: "缓慢沉重的大幅挥击", Frames: 6, Loop: false, PromptText: "heavy attack animation, long exaggerated wind-up loading weight back, then a slow powerful strike", Choreography: "Heavy attack: long exaggerated wind-up loading weight back, a slow powerful swing, deep follow-through, slow recovery. Bigger and slower than a normal attack."}
	ActionUppercut    = ActionPreset{ID: "uppercut", Name: "上勾拳", Category: ActionCategoryCombat, Description: "自下而上的升龙拳击", Frames: 4, Loop: false, PromptText: "uppercut animation, dip the body low loading the legs, then drive upward exploding the fist up through the target", Choreography: "Uppercut: dip the body low loading the legs, drive upward exploding the fist up through the target, finish with body extended tall. Peak frame reaching upward."}
	ActionParry       = ActionPreset{ID: "parry", Name: "格挡弹反", Category: ActionCategoryCombat, Description: "格开来袭攻击的反击预备", Frames: 4, Loop: false, PromptText: "parry animation, a sharp deflecting flick of the weapon or arm to one side that knocks an incoming attack away", Choreography: "Parry: a sharp deflecting flick of the weapon or arm to one side that knocks an attack away, then snap back to ready. Quick and crisp."}
	ActionReload      = ActionPreset{ID: "reload", Name: "装弹", Category: ActionCategoryCombat, Description: "远程武器换弹", Frames: 5, Loop: false, PromptText: "reload animation, lower the weapon, work the mechanism with the off hand, eject and insert fresh ammo, ready again", Choreography: "Reload sequence: lower the weapon, work the mechanism with the off hand (eject, insert, seat), and raise back to ready. Hands do the distinct work across frames."}
	ActionFall        = ActionPreset{ID: "fall", Name: "坠落", Category: ActionCategoryMovement, Description: "空中坠落四肢摆动", Frames: 4, Loop: true, PromptText: "fall animation, body airborne, arms and legs flailing or bracing, slight rotation as it drops", Choreography: "Falling cycle: body airborne, arms and legs flailing or bracing, slight rotation or wobble, hair and clothes pushed upward by wind. No ground contact in any frame."}
	ActionLand        = ActionPreset{ID: "land", Name: "落地", Category: ActionCategoryMovement, Description: "从高处落地屈膝缓冲", Frames: 4, Loop: false, PromptText: "land animation, feet touch down, deep knee bend to absorb the impact, body compresses then straightens", Choreography: "Landing impact: feet touch down, deep knee bend to absorb shock, body compresses low, then rises back toward standing. Show the compression clearly in the middle frame."}
	ActionDrawWeapon  = ActionPreset{ID: "draw-weapon", Name: "拔武器", Category: ActionCategoryCombat, Description: "拔出武器进入预备姿势", Frames: 5, Loop: false, PromptText: "draw weapon animation, reach for the weapon, pull it free in a sweeping motion, settle into a ready stance", Choreography: "Draw weapon: reach for the weapon, pull it free in a sweeping motion, settle into a ready combat stance. Final frame is the ready pose with weapon up."}
	ActionGuardBreak  = ActionPreset{ID: "guard-break", Name: "破防", Category: ActionCategoryDamage, Description: "防御被砸开后仰", Frames: 4, Loop: false, PromptText: "guard break animation, the raised guard is smashed open, arms fly apart, body knocked back off balance", Choreography: "Guard break: the raised guard is smashed open, arms fly apart, body rocks backward off balance, briefly exposed. Recoil reads as defense failing."}
	ActionCounter     = ActionPreset{ID: "counter", Name: "反击", Category: ActionCategoryCombat, Description: "先受身后立即反击", Frames: 5, Loop: false, PromptText: "counter animation, a tight defensive flinch, then an immediate sharp counterattack in return", Choreography: "Counter: a tight defensive flinch, then an immediate sharp counterattack exploding forward at full extension, then recovery. Two-beat defense-into-offense."}
	ActionDrink2      = ActionPreset{ID: "drink-potion", Name: "喝药", Category: ActionCategoryUtility, Description: "仰头喝下药水瓶", Frames: 5, Loop: false, PromptText: "drink potion animation, raise a small bottle to the mouth, tip the head back to drink, lower the bottle with a satisfied breath", Choreography: "Drinking: raise a hand to the mouth as if holding a small bottle, tip the head back to drink, then lower the bottle with a satisfied breath. Shoulders relax on the final frame."}
	ActionRead        = ActionPreset{ID: "read", Name: "阅读", Category: ActionCategoryUtility, Description: "捧书低头阅读", Frames: 4, Loop: true, PromptText: "read animation, both hands held out front as if holding an open book, head tilted down scanning the page", Choreography: "Reading loop: both hands held out front as if holding an open book, head tilted down scanning, occasional small head shift or page turn. Looping calm study."}
	ActionThink       = ActionPreset{ID: "think", Name: "思考", Category: ActionCategoryEmotion, Description: "手托下巴思考", Frames: 4, Loop: true, PromptText: "think animation, one hand to the chin, head tilted, weight shifting slowly side to side", Choreography: "Thinking loop: one hand to the chin, head tilted, weight shifting slowly side to side, occasional small head tilt. Pondering, looping subtle motion."}
)

// ActionPresets returns the built-in action presets in definition order.
func ActionPresets() []ActionPreset {
	return []ActionPreset{
		ActionIdle, ActionWalk, ActionRun, ActionJump, ActionRoll, ActionDash, ActionCrouch,
		ActionCrawl, ActionClimb, ActionSwim, ActionSit, ActionSleep, ActionTurn, ActionFall, ActionLand,
		ActionAttack, ActionHeavyAttack, ActionSlash, ActionStab, ActionPunch, ActionUppercut, ActionKick,
		ActionBlock, ActionParry, ActionDodge, ActionBackstep, ActionShoot, ActionReload, ActionThrow,
		ActionCombo, ActionPowerUp, ActionSpinAttack, ActionDrawWeapon, ActionCounter, ActionGuardBreak, ActionTaunt,
		ActionHurt, ActionKnockback, ActionKnockdown, ActionGetUp, ActionStun, ActionDeath, ActionDeathFall,
		ActionRevive, ActionLowHP,
		ActionCast, ActionCastFire, ActionCastIce, ActionCastHeal, ActionChannel, ActionShield, ActionSummon, ActionMeditate,
		ActionWave, ActionCheer, ActionBow, ActionNod, ActionDance, ActionVictory, ActionLaugh, ActionCry,
		ActionAngry, ActionSurprised, ActionSad, ActionPoint, ActionSalute, ActionThink,
		ActionPickUp, ActionPush, ActionPull, ActionOpen, ActionEat, ActionDrink, ActionDrink2, ActionDig, ActionRead,
	}
}

// ActionPresetByID looks up an action preset by id.
func ActionPresetByID(id string) (ActionPreset, error) {
	for _, p := range ActionPresets() {
		if p.ID == id {
			return p, nil
		}
	}
	return ActionPreset{}, fmt.Errorf("pipeline: unknown action preset %q", id)
}

// ActionPresetFrames returns the suggested frame count for an action preset
// (0 表示未知，由调用方兜底默认值)。前端新建动作时用它设置目标帧数。
func ActionPresetFrames(id string) int {
	p, err := ActionPresetByID(id)
	if err != nil {
		return 0
	}
	return p.Frames
}
