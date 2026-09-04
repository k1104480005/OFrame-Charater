package pipeline

// FacingSection 逐字对齐 perfectpixel internal/sprite/direction.go 的
// FacingPromptSection：按生成方向给出方向锁段落（视图/机位/身体朝向/可见
// 部位），并要求全条带每一帧使用同一视角。方向 token 是本工作台的九宫格
// 词表（down = 正面），与 perfectpixel 的罗盘词表一一对应；镜像派生方向
// （left 等）本地翻转、不调用 provider，但点亮式独立生成时同样需要锁。
func FacingSection(direction string) string {
	d, ok := facingDescs[direction]
	if !ok {
		return ""
	}
	return "Facing direction lock (overrides any other facing or view instruction in this prompt):\n" +
		"- Required view: " + d.view + " — " + d.camera + ".\n" +
		"- Body orientation: " + d.body + ".\n" +
		"- Visibility: " + d.visibility + ".\n" +
		"- The attached reference image shows this character from the front; redraw the IDENTICAL character (same hair, outfit, colors, proportions) rotated to this view.\n" +
		"- Every frame in the strip must use this exact same viewing angle. Never drift back toward a front view and never mirror the character between frames.\n"
}

type facingDesc struct {
	view       string
	camera     string
	body       string
	visibility string
}

var facingDescs = map[string]facingDesc{
	// down = 罗盘 south（正面）。
	"down": {
		view:       "front view",
		camera:     "camera directly in front, at eye level",
		body:       "the character faces the viewer directly",
		visibility: "full face visible (eyes and mouth, minimal nose detail); both arms and both legs fully visible, symmetric",
	},
	// up = 罗盘 north（背面）。
	"up": {
		view:       "back view",
		camera:     "camera positioned directly behind the character",
		body:       "the character faces away from the viewer",
		visibility: "face completely hidden, only the back of the head and hair visible; back of the outfit, both arms and legs seen from behind",
	},
	// right = 罗盘 east。
	"right": {
		view:       "right-side profile view",
		camera:     "camera at the character's right side, perpendicular to the body; strictly 2D profile, no perspective rotation",
		body:       "the character faces and moves toward the RIGHT edge of the canvas",
		visibility: "right profile of the face only (one eye, one ear); right arm and right leg prominent, left limbs fully hidden behind the body; never show parts of the left side",
	},
	// left = 罗盘 west（right 的镜像视图，独立生成时同样需要锁）。
	"left": {
		view:       "left-side profile view",
		camera:     "camera at the character's left side, perpendicular to the body; strictly 2D profile, no perspective rotation",
		body:       "the character faces and moves toward the LEFT edge of the canvas",
		visibility: "left profile of the face only (one eye, one ear); left arm and left leg prominent, right limbs fully hidden behind the body; never show parts of the right side",
	},
	// down-right = 罗盘 south-east。
	"down-right": {
		view:       "three-quarter front-right view",
		camera:     "camera at front-right, rotated about 45 degrees from straight ahead",
		body:       "the character is turned about 45 degrees to the right, mostly facing the viewer",
		visibility: "3/4 face with both eyes visible, right side emphasized; right arm and leg fully visible, left side partially visible",
	},
	// down-left = 罗盘 south-west（down-right 的镜像视图）。
	"down-left": {
		view:       "three-quarter front-left view",
		camera:     "camera at front-left, rotated about 45 degrees from straight ahead",
		body:       "the character is turned about 45 degrees to the left, mostly facing the viewer",
		visibility: "3/4 face with both eyes visible, left side emphasized; left arm and leg fully visible, right side partially visible",
	},
	// up-right = 罗盘 north-east。
	"up-right": {
		view:       "three-quarter back-right view",
		camera:     "camera behind and to the right, rotated about 45 degrees",
		body:       "the character is turned away from the viewer, showing the back-right side",
		visibility: "face hidden except a hint of the right jaw; back and right shoulder prominent, right arm and leg visible from behind",
	},
	// up-left = 罗盘 north-west（up-right 的镜像视图）。
	"up-left": {
		view:       "three-quarter back-left view",
		camera:     "camera behind and to the left, rotated about 45 degrees",
		body:       "the character is turned away from the viewer, showing the back-left side",
		visibility: "face hidden except a hint of the left jaw; back and left shoulder prominent, left arm and leg visible from behind",
	},
}
