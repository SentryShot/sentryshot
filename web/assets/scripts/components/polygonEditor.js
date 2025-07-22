// @ts-check

/**
 * @typedef {[number, number]} Point
 * @typedef {Point[]} Points
 */

/**
 * @callback OnChangeFunc
 * @param {number} index
 * @param {number} x
 * @param {number} y
 */

/**
 * @typedef {Object} PolygonEditorProps
 * @property {string=} color
 * @property {number=} opacity
 * @property {number=} stepSize
 * @property {OnChangeFunc=} onChange
 * @property {boolean=} visible
 */

/**
 * @param {Element} element
 * @param {PolygonEditorProps} props
 */
function newPolygonEditor(element, props) {
	const onChange = props.onChange;
	let { color, opacity, stepSize, visible } = props;
	if (color === undefined) {
		color = "black";
	}
	if (opacity === undefined) {
		opacity = 0.2;
	}
	if (stepSize === undefined) {
		stepSize = 1;
	}
	if (visible === undefined) {
		visible = true;
	}

	let enabled = true;
	let selected = 0;
	/** @type {Points} */
	let value;

	const resegment = () => {
		if (value.length <= 3) {
			return false;
		}
		/** @type {number[]} */
		for (let i = 0; i < value.length; i++) {
			const j = (i + 1) % value.length;
			const k = (i + 2) % value.length;

			if (j === selected) {
				continue;
			}

			const x1 = value[i][0];
			const y1 = value[i][1];
			const x2 = value[j][0];
			const y2 = value[j][1];
			const x3 = value[k][0];
			const y3 = value[k][1];

			const AB = Math.hypot(x2 - x1, y2 - y1);
			const BC = Math.hypot(x2 - x3, y2 - y3);
			const AC = Math.hypot(x3 - x1, y3 - y1);

			const angle = Math.acos((BC * BC + AB * AB - AC * AC) / (2 * BC * AB));
			const degreesFrom180 = Math.abs((angle * 180) / Math.PI - 180);

			const minSegmentDegrees = 10;
			if (degreesFrom180 < minSegmentDegrees) {
				// Only remove one point at a time.
				value.splice(j, 1);
				if (j < selected) {
					selected = (selected + value.length - 1) % value.length;
				}
				return true;
			}
		}
		return false;
	};

	const arrowStyle = `style="fill: var(--color0); opacity: 0.85"`;
	const scale = Number(
		window.getComputedStyle(document.body).getPropertyValue("--scale"),
	);
	const pointRadius = `${scale * 0.15}rem`;
	const fakePointRadius = `${scale * 0.1}rem`;
	const render = () => {
		let html = "";

		if (visible) {
			// Area.
			let points = "";
			for (const [x, y] of value) {
				points += `${x},${y} `;
			}
			html += `<polygon points="${points}" style="fill: ${color}; opacity: ${opacity};"></polygon>`;
		}

		if (visible && enabled) {
			// Points.
			for (const [i, [x, y]] of value.entries()) {
				if (i !== selected) {
					html += `<circle cx="${x}" cy="${y}" r="${pointRadius}" data="${i}" ${arrowStyle} class="js-point"></circle>`;
				}
			}

			// Fake points.
			for (let i = 0; i < value.length; i++) {
				const j = (i + 1) % value.length;
				const x = (value[i][0] + value[j][0]) / 2;
				const y = (value[i][1] + value[j][1]) / 2;
				html += `<circle cx="${x}" cy="${y}" r="${fakePointRadius}" data="${i}" ${arrowStyle} class="js-fake-point"></circle>`;
			}

			// Arrows.
			let rem = Number.parseFloat(
				getComputedStyle(document.documentElement).fontSize,
			);
			if (!rem) {
				rem = 16;
			}
			const offset = 0.2 * rem;
			const width = offset * 0.75;
			const height = width * 1.5;
			const [x, y] = value[selected];

			// Top.
			const top1 = `${x},${y - offset - height}`;
			const top2 = `${x - width},${y - offset}`;
			const top3 = `${x + width},${y - offset}`;
			html += `<polygon points="${top1} ${top2} ${top3}" ${arrowStyle} class="js-top"></polygon>`;
			// Left.
			const left1 = `${x - offset - height},${y}`;
			const left2 = `${x - offset},${y - width}`;
			const left3 = `${x - offset},${y + width}`;
			html += `<polygon points="${left1} ${left2} ${left3}" ${arrowStyle} class="js-left"></polygon>`;
			// Bottom.
			const bottom1 = `${x},${y + offset + height}`;
			const bottom2 = `${x - width},${y + offset}`;
			const bottom3 = `${x + width},${y + offset}`;
			html += `<polygon points="${bottom1} ${bottom2} ${bottom3}" ${arrowStyle} class="js-bottom"></polygon>`;
			// Right.
			const right1 = `${x + offset + height},${y}`;
			const right2 = `${x + offset},${y - width}`;
			const right3 = `${x + offset},${y + width}`;
			html += `<polygon points="${right1} ${right2} ${right3}" ${arrowStyle} class="js-right"></polygon>`;
		}

		element.innerHTML = html;

		if (onChange && enabled) {
			const [x, y] = value[selected];
			onChange(selected, x, y);
		}
	};

	/** @param {number} i */
	const selectPoint = (i) => {
		selected = i;
		while (resegment()) {
			/* empty */
		}
	};

	/** @param {number} i */
	const createPoint = (i) => {
		const j = (i + 1) % value.length;
		const x = Math.round((value[i][0] + value[j][0]) / 2);
		const y = Math.round((value[i][1] + value[j][1]) / 2);
		value.splice(j, 0, [x, y]);
		selected = j;
		while (resegment()) {
			/* empty */
		}
	};

	element.addEventListener("click", (e) => {
		const target = e.target;
		if (target instanceof SVGElement) {
			switch (target.getAttribute("class")) {
				case "js-top": {
					value[selected][1] = Math.max(0, value[selected][1] - stepSize);
					break;
				}
				case "js-left": {
					value[selected][0] = Math.max(0, value[selected][0] - stepSize);
					break;
				}
				case "js-bottom": {
					value[selected][1] = Math.min(100, value[selected][1] + stepSize);
					break;
				}
				case "js-right": {
					value[selected][0] = Math.min(100, value[selected][0] + stepSize);
					break;
				}
				case "js-point": {
					selectPoint(Number(target.getAttribute("data")));
					break;
				}
				case "js-fake-point": {
					createPoint(Number(target.getAttribute("data")));
					break;
				}
				default:
			}
			render();
		}
		e.stopPropagation();
	});

	return {
		/** @param {Points} v */
		set(v) {
			value = v;
			render();
		},
		/**
		 * @param {number} i
		 * @param {number} x
		 * @param {number} y
		 */
		setIndex(i, x, y) {
			value[i] = [x, y];
			render();
		},
		value() {
			return value;
		},
		/** @param {number} i */
		selectPoint(i) {
			selectPoint(i);
		},
		/** @param {number} i */
		createPoint(i) {
			createPoint(i);
		},
		selected() {
			return selected;
		},
		/** @param {string} v */
		setColor(v) {
			color = v;
		},
		/** @param {number} v */
		setStepSize(v) {
			stepSize = v;
		},
		/** @param {boolean} v */
		enabled(v) {
			enabled = v;
			render();
		},
		isEnabled() {
			return enabled;
		},
		/** @param {boolean} v */
		visible(v) {
			visible = v;
			render();
		},
		isVisible() {
			return visible;
		},
	};
}

export { newPolygonEditor };
