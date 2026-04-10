package analysis

import (
	"context"
	"time"

	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/resultmeta"
)

func qualitativeAnalyzerWithTimings(ctx context.Context, analyzer qualitativeAnalyzer) qualitativeAnalyzer {
	tracker := resultmeta.TimingTrackerFromContext(ctx)
	if analyzer == nil || tracker == nil {
		return analyzer
	}
	return timedQualitativeAnalyzer{inner: analyzer, tracker: tracker}
}

func complianceAdjudicatorWithTimings(ctx context.Context, adjudicator complianceAdjudicator) complianceAdjudicator {
	tracker := resultmeta.TimingTrackerFromContext(ctx)
	if adjudicator == nil || tracker == nil {
		return adjudicator
	}
	return timedComplianceAdjudicator{inner: adjudicator, tracker: tracker}
}

func impactSeverityClassifierWithTimings(ctx context.Context, classifier impactSeverityClassifier) impactSeverityClassifier {
	tracker := resultmeta.TimingTrackerFromContext(ctx)
	if classifier == nil || tracker == nil {
		return classifier
	}
	return timedImpactSeverityClassifier{inner: classifier, tracker: tracker}
}

func metadataInferrerWithTimings(ctx context.Context, inferrer metadataInferrer) metadataInferrer {
	tracker := resultmeta.TimingTrackerFromContext(ctx)
	if inferrer == nil || tracker == nil {
		return inferrer
	}
	return timedMetadataInferrer{inner: inferrer, tracker: tracker}
}

func embedderWithTimings(ctx context.Context, embedder index.Embedder) index.Embedder {
	tracker := resultmeta.TimingTrackerFromContext(ctx)
	if embedder == nil || tracker == nil {
		return embedder
	}
	return timedEmbedder{inner: embedder, tracker: tracker}
}

type timedQualitativeAnalyzer struct {
	inner   qualitativeAnalyzer
	tracker *resultmeta.TimingTracker
}

func (t timedQualitativeAnalyzer) Probe(ctx context.Context) error {
	start := time.Now()
	err := t.inner.Probe(ctx)
	t.tracker.AddAnalysis(time.Since(start), 1)
	return err
}

func (t timedQualitativeAnalyzer) Compare(ctx context.Context, orderedRefs []string, specs map[string]specDocument, base Comparison) (Comparison, error) {
	start := time.Now()
	result, err := t.inner.Compare(ctx, orderedRefs, specs, base)
	t.tracker.AddAnalysis(time.Since(start), 1)
	return result, err
}

func (t timedQualitativeAnalyzer) RefineDocDrift(ctx context.Context, doc docDocument, specs map[string]specDocument, item DriftItem, remediation *DocRemediationItem) (*DriftItem, *DocRemediationItem, error) {
	start := time.Now()
	refined, remediated, err := t.inner.RefineDocDrift(ctx, doc, specs, item, remediation)
	t.tracker.AddAnalysis(time.Since(start), 1)
	return refined, remediated, err
}

type timedComplianceAdjudicator struct {
	inner   complianceAdjudicator
	tracker *resultmeta.TimingTracker
}

func (t timedComplianceAdjudicator) AdjudicateCompliance(ctx context.Context, request complianceAdjudicationRequest) (*complianceAdjudicationResponse, error) {
	start := time.Now()
	result, err := t.inner.AdjudicateCompliance(ctx, request)
	t.tracker.AddAnalysis(time.Since(start), 1)
	return result, err
}

type timedImpactSeverityClassifier struct {
	inner   impactSeverityClassifier
	tracker *resultmeta.TimingTracker
}

func (t timedImpactSeverityClassifier) ClassifyImpactSeverity(ctx context.Context, request impactSeverityRequest) (*impactSeverityResponse, error) {
	start := time.Now()
	result, err := t.inner.ClassifyImpactSeverity(ctx, request)
	t.tracker.AddAnalysis(time.Since(start), 1)
	return result, err
}

type timedMetadataInferrer struct {
	inner   metadataInferrer
	tracker *resultmeta.TimingTracker
}

func (t timedMetadataInferrer) InferMetadata(ctx context.Context, request metadataInferenceRequest) (*MetadataInferenceResult, error) {
	start := time.Now()
	result, err := t.inner.InferMetadata(ctx, request)
	t.tracker.AddAnalysis(time.Since(start), 1)
	return result, err
}

type timedEmbedder struct {
	inner   index.Embedder
	tracker *resultmeta.TimingTracker
}

func (t timedEmbedder) Fingerprint() string {
	return t.inner.Fingerprint()
}

func (t timedEmbedder) Dimension(ctx context.Context) (int, error) {
	start := time.Now()
	dimension, err := t.inner.Dimension(ctx)
	t.tracker.AddEmbedding(time.Since(start), 1)
	return dimension, err
}

func (t timedEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float64, error) {
	start := time.Now()
	vectors, err := t.inner.EmbedDocuments(ctx, texts)
	t.tracker.AddEmbedding(time.Since(start), 1)
	return vectors, err
}

func (t timedEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float64, error) {
	start := time.Now()
	vectors, err := t.inner.EmbedQueries(ctx, texts)
	t.tracker.AddEmbedding(time.Since(start), 1)
	return vectors, err
}
