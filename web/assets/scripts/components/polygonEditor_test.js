import { newPolygonEditor } from "./polygonEditor.js";

test("ok", () => {
	document.body.innerHTML = `<svg></svg>`;
	const element = document.querySelector("svg");

	const editor = newPolygonEditor(element, "black");
	const input = [
		[20, 20],
		[80, 20],
		[50, 70],
	];
	editor.set(input);

	expect(editor.isEnabled()).toBe(true);
	editor.enabled(false);
	expect(editor.isEnabled()).toBe(false);
	editor.enabled(true);
	expect(editor.isEnabled()).toBe(true);

	expect(editor.isVisible()).toBe(true);
	editor.visible(false);
	expect(editor.isVisible()).toBe(false);
	editor.visible(true);
	expect(editor.isVisible()).toBe(true);

	const enabled = `
		<svg>
			<polygon points="20,20 80,20 50,70" style="fill: black; opacity: 0.2;"></polygon>
			<circle cx="80" cy="20" r="0.045rem" data="1" style="fill: var(--colorbg); opacity: 0.85" class="js-point"></circle>
			<circle cx="50" cy="70" r="0.045rem" data="2" style="fill: var(--colorbg); opacity: 0.85" class="js-point"></circle>
			<circle cx="50" cy="20" r="0.03rem" data="0" style="fill: var(--colorbg); opacity: 0.85" class="js-fake-point"></circle>
			<circle cx="65" cy="45" r="0.03rem" data="1" style="fill: var(--colorbg); opacity: 0.85" class="js-fake-point"></circle>
			<circle cx="35" cy="45" r="0.03rem" data="2" style="fill: var(--colorbg); opacity: 0.85" class="js-fake-point"></circle>
			<polygonpoints="20,17.279999999999998 19.04,18.72 20.96,18.72" style="fill: var(--colorbg); opacity: 0.85" class="js-top"></polygon>
			<polygonpoints="17.279999999999998,20 18.72,19.04 18.72,20.96" style="fill: var(--colorbg); opacity: 0.85" class="js-left"></polygon>
			<polygonpoints="20,22.720000000000002 19.04,21.28 20.96,21.28" style="fill: var(--colorbg); opacity: 0.85" class="js-bottom"></polygon>
			<polygonpoints="22.720000000000002,20 21.28,19.04 21.28,20.96" style="fill: var(--colorbg); opacity: 0.85" class="js-right"></polygon>
		</svg>`.replaceAll(/\s/g, "");

	const got = document.body.innerHTML.replaceAll(/\s/g, "");
	expect(got).toBe(enabled);
	expect(editor.value()).toBe(input);

	editor.enabled(false);
	const got2 = document.body.innerHTML.replaceAll(/\s/g, "");
	const disabled = `
		<svg>
			<polygon points="20,20 80,20 50,70" style="fill: black; opacity: 0.2;"></polygon>
		</svg>`.replaceAll(/\s/g, "");
	expect(got2).toBe(disabled);
	expect(editor.value()).toBe(input);

	editor.visible(false);
	const got3 = document.body.innerHTML.replaceAll(/\s/g, "");
	const hidden = `<svg></svg>`.replaceAll(/\s/g, "");
	expect(got3).toBe(hidden);
	expect(editor.value()).toBe(input);
});

test("resegment", () => {
	document.body.innerHTML = `<svg></svg>`;
	const element = document.querySelector("svg");

	const editor = newPolygonEditor(element, "black");
	const A = [20, 20];
	const B = [80, 20];
	const C = [50, 70];
	editor.set([A, B, C]);

	// Create and select mid point.
	editor.createPoint(1);
	expect(editor.value()).toEqual([A, B, [65, 45], C]);

	// Deselect mid point.
	editor.selectPoint(0);
	expect(editor.value()).toEqual([A, B, C]);
});

test("resegment2", () => {
	document.body.innerHTML = `<svg></svg>`;
	const element = document.querySelector("svg");

	const editor = newPolygonEditor(element, "black");
	const A = [20, 20];
	const B = [80, 20];
	const C = [50, 70];
	editor.set([A, B, C]);

	// Create mid point.
	editor.createPoint(0);
	expect(editor.value()).toEqual([A, [50, 20], B, C]);

	// Raise mid point.
	editor.setIndex(1, 50, 17);

	// Create left-mid point.
	editor.createPoint(0);
	expect(editor.value()).toEqual([A, [35, 19], [50, 17], B, C]);

	// Lower left-mid point.
	editor.setIndex(1, 35, 20);

	// Create right-mid point.
	editor.createPoint(2);
	expect(editor.value()).toEqual([A, [35, 20], [50, 17], [65, 19], B, C]);

	// Lower right-mid point.
	editor.setIndex(3, 65, 20);

	// Select midpoint.
	editor.selectPoint(2);
	expect(editor.value()).toEqual([A, [35, 20], [50, 17], [65, 20], B, C]);

	// Lower mid point.
	expect(editor.selected()).toBe(2);
	editor.setIndex(2, 50, 20);
	expect(editor.value()).toEqual([A, [35, 20], [50, 20], [65, 20], B, C]);

	// Deselect mid point.
	editor.selectPoint(0);
	expect(editor.value()).toEqual([A, B, C]);
	expect(editor.selected()).toBe(0);
});

test("resegment3", () => {
	document.body.innerHTML = `<svg></svg>`;
	const element = document.querySelector("svg");

	const editor = newPolygonEditor(element, "black");
	editor.set([
		[20, 20],
		[80, 20],
		[50, 70],
	]);

	// Move bottom point between the 2 top poitns.
	editor.setIndex(2, 50, 20);
	editor.selectPoint(0);

	// Should not have resegmented.
	expect(editor.value()).toEqual([
		[20, 20],
		[80, 20],
		[50, 20],
	]);
});
