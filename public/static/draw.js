class LineDrawer {
    constructor(canvasId, outputId) {
        this.canvas = document.getElementById(canvasId);
        this.output = document.getElementById(outputId);
        this.ctx = this.canvas.getContext("2d");
        this.canvas.width = document.getElementById("controls").offsetWidth-100;
        this.canvas.height = document.getElementById("controls").offsetHeight - 150;
        this.output.style.width = `${document.getElementById("controls").offsetWidth - 100}px`;
        this.drawing = false;
        this.points = [];
        this.resamplePoints = [];
        this.bindEvents();
    }

    bindEvents() {
        this.canvas.addEventListener("pointerdown", (e) => {
            e.preventDefault();
            this.drawing = true;
            this.points = [{ x: e.offsetX, y: e.offsetY }];
            this.updateText();
            return false;
        },{passive:false});

        this.canvas.addEventListener("pointermove", (e) => {
            e.preventDefault();
            if (!this.drawing) return;
            // if (e.buttons!=1) return;
            this.points.push({ x: e.offsetX, y: e.offsetY });
            this.redraw();
            this.updateText();
            return false;
        },{passive:false});

        this.canvas.addEventListener("pointerup", (e) => {
            e.preventDefault();
            this.drawing = false;
            return false;
        });
    }

    getXYPoints() {
        // const step = this.canvas.width / Math.max(1, this.points.length - 1);
        // return this.points.map((y, i) => ({ x: i * step, y }));
        return this.points;
    }

    redraw() {
        let points=this.getXYPoints()
        const ctx = this.ctx;
        ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
        if (points.length < 2) return;

        ctx.beginPath();
        ctx.moveTo(points[0].x, points[0].y);
        for (let i = 1; i < points.length - 2; i++) {
            const xc = (points[i].x + points[i + 1].x) / 2;
            const yc = (points[i].y + points[i + 1].y) / 2;
            ctx.quadraticCurveTo(points[i].x, points[i].y, xc, yc);
        }
        ctx.strokeStyle = "#000";
        ctx.lineWidth = 2;
        ctx.stroke();
    }

    simplifyLine() {
        let points=this.getXYPoints();
        if (points.length <= 30) {
            return
        }
        if (points.length <= 100) {
            this.updateText(points);
            return;
        }

        const simplified = this.rdp(points, 2);
        const resampled = this.resample(simplified, 100);
        this.updateText(resampled);

        const ctx = this.ctx;
        ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
        ctx.beginPath();
        ctx.moveTo(resampled[0].x, resampled[0].y);
        for (let i = 1; i < resampled.length; i++) {
            ctx.lineTo(resampled[i].x, resampled[i].y);
        }
        ctx.strokeStyle = "red";
        ctx.lineWidth = 2;
        ctx.stroke();
    }

    updateText(pts = this.getXYPoints()) {
        const lines = pts.map(p => {
            const flippedY = this.canvas.height - p.y;
            return `x: ${p.x.toFixed(2)}, y: ${flippedY.toFixed(2)}`;
        });
        this.output.value = lines.join('\n');
    }

    rdp(points, epsilon) {
        if (points.length < 3) return points;

        const getPerpendicularDistance = (p, p1, p2) => {
            const area = Math.abs(
                0.5 * (p1.x * p2.y + p2.x * p.y + p.x * p1.y -
                    p2.x * p1.y - p.x * p2.y - p1.x * p.y)
            );
            const bottom = Math.hypot(p1.x - p2.x, p1.y - p2.y);
            return (area * 2) / bottom;
        };

        let dmax = 0;
        let index = 0;
        for (let i = 1; i < points.length - 1; i++) {
            const d = getPerpendicularDistance(points[i], points[0], points[points.length - 1]);
            if (d > dmax) {
                index = i;
                dmax = d;
            }
        }

        if (dmax > epsilon) {
            const rec1 = this.rdp(points.slice(0, index + 1), epsilon);
            const rec2 = this.rdp(points.slice(index), epsilon);
            return rec1.slice(0, -1).concat(rec2);
        } else {
            return [points[0], points[points.length - 1]];
        }
    }

    resample(pts, n) {
        let totalDist = 0;
        const dists = [0];
        for (let i = 1; i < pts.length; i++) {
            const d = Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y);
            totalDist += d;
            dists.push(totalDist);
        }

        const interval = totalDist / (n - 1);
        const newPts = [pts[0]];
        let targetDist = interval;
        let j = 1;

        for (let i = 1; i < n - 1; i++) {
            while (dists[j] < targetDist && j < dists.length - 1) j++;
            const t = (targetDist - dists[j - 1]) / (dists[j] - dists[j - 1]);
            const x = pts[j - 1].x + t * (pts[j].x - pts[j - 1].x);
            const y = pts[j - 1].y + t * (pts[j].y - pts[j - 1].y);
            newPts.push({ x, y });
            targetDist += interval;
        }

        newPts.push(pts[pts.length - 1]);
        this.resamplePoints = newPts;
        return newPts;
    }


    // 提取形态序列 (标准化)
    extractPattern(prices) {
        const segment = prices.slice();
        const minPrice = Math.min(...segment);
        const maxPrice = Math.max(...segment);

        // 标准化到0-1范围
        return segment.map(price => (price - minPrice) / (maxPrice - minPrice));
    }

    // 计算双顶相似度
    calculateSimilarity(series) {
        // 标准双顶形态模板 (M形)
        let that=this.canvas.height;
        let yresampe=this.resamplePoints.map(sample=> that - sample.y)
        const doubleTopTemplate = this.extractPattern(yresampe);

        // 如果序列太短，返回0
        if (series.length < 10) return 0;

        // 计算DTW距离
        const dtwMatrix = [];

        // 初始化矩阵
        for (let i = 0; i <= series.length; i++) {
            dtwMatrix[i] = [];
            for (let j = 0; j <= doubleTopTemplate.length; j++) {
                if (i === 0 && j === 0) {
                    dtwMatrix[i][j] = 0;
                } else if (i === 0 || j === 0) {
                    dtwMatrix[i][j] = Infinity;
                } else {
                    const cost = Math.abs(series[i - 1] - doubleTopTemplate[j - 1]);
                    dtwMatrix[i][j] = cost + Math.min(dtwMatrix[i - 1][j], dtwMatrix[i][j - 1], dtwMatrix[i - 1][j - 1]);
                }
            }
        }

        // 获取DTW距离
        const distance = dtwMatrix[series.length][doubleTopTemplate.length];

        // 转换为相似度 (0-1)
        return 1 / (1 + distance / doubleTopTemplate.length);
    }

}