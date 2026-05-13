// 渲染结果
function renderResults1(results, requireBreakout) {
    results.sort((a, b) => {
        // 获取最高相似度
        const maxSimA = Math.max(...a.patterns.map(p => p.similarity));
        const maxSimB = Math.max(...b.patterns.map(p => p.similarity));
        return maxSimB - maxSimA; // 降序排序
    });
    results.forEach(result => {
        // 如果要求颈线突破但未突破，则跳过
        if (requireBreakout && !result.necklineBroken) return;

        // // 为每个交易对找到相似度最高的形态
        // const bestPattern = result.patterns.reduce((best, current) => {
        //     return (current.similarity > best.similarity) ? current : best;
        // });

        // // 确定匹配级别
        // const similarity = bestPattern.similarity;
        // const matchClass = similarity >= 85 ? 'high-match' : similarity >= 70 ? 'medium-match' : '';



    });
}
